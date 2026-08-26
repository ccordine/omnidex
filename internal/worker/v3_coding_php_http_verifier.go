package worker

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func phpServiceHTTPVerifierDocument(
	bindings []phpServiceFeatureBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	state directCodingServiceStatePlan,
) (assemblyline.SourceDocument, error) {
	lifecycle, err := derivePHPServiceHTTPLifecyclePlan(
		workload, capabilities, state, bindings,
	)
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	source, err := phpServiceHTTPVerifierSource(bindings, lifecycle)
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	return assemblyline.SourceDocument{
		ID: "application_http_verifier", Path: "tests/HttpVerifier.php", AdapterID: phpSourceAdapterID,
		Preamble: "<?php\ndeclare(strict_types=1);",
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.http.verify", Static: source,
			API: "make bounded consumer HTTP requests through NGINX and fail on transport or durable lifecycle mismatch",
		}},
	}, nil
}

func phpServiceHTTPVerifierSource(
	bindings []phpServiceFeatureBinding,
	lifecycle phpServiceHTTPLifecyclePlan,
) (string, error) {
	var checks strings.Builder
	checks.WriteString("verifyHttpResponse(performHttpRequest('GET', '/__omnidex/health', [], ''), 204, 'none');\n")
	lifecycleSource, err := phpServiceHTTPLifecycleSource(lifecycle)
	if err != nil {
		return "", err
	}
	checks.WriteString(lifecycleSource)
	for _, binding := range bindings {
		if !binding.HasEndpoint {
			continue
		}
		path, headers, body := phpServiceHTTPWireFixture(binding)
		checks.WriteString(fmt.Sprintf(
			"verifyHttpResponse(performHttpRequest(%s, %s, %s, %s), %d, %s);\n",
			phpSingleQuoted(binding.Fixture.Method), phpSingleQuoted(path),
			phpServiceFixtureArray(headers), phpSingleQuoted(body),
			binding.Endpoint.SuccessStatus, phpSingleQuoted(string(binding.Endpoint.ResponseMedia)),
		))
		checks.WriteString(fmt.Sprintf(
			"verifyHttpResponse(performHttpRequest('OPTIONS', %s, [], ''), 405, 'text/plain');\n",
			phpSingleQuoted(path),
		))
	}
	checks.WriteString(fmt.Sprintf(
		"verifyHttpResponse(performHttpRequest('GET', %s, [], ''), 404, 'text/plain');\n",
		phpSingleQuoted(phpServiceUnmatchedRoute(bindings)),
	))
	if phpServiceBindingsHaveHTML(bindings) {
		checks.WriteString("verifyStylesheetResponse(performHttpRequest('GET', '/assets/app.css', [], ''));\n")
	}
	return phpServiceHTTPVerifierRuntime() + "\n\n" + phpServiceHTTPLifecycleRuntime() + "\n\ntry {\n" +
		indentPHPSource(checks.String(), "    ") +
		"} catch (Throwable $failure) {\n" +
		"    fwrite(STDERR, 'HTTP transport verification failed: ' . $failure->getMessage() . PHP_EOL);\n" +
		"    exit(1);\n" +
		"}\n" +
		"fwrite(STDOUT, 'HTTP transport verification passed.' . PHP_EOL);", nil
}

func phpServiceHTTPWireFixture(
	binding phpServiceFeatureBinding,
) (string, []phpServiceFixturePair, string) {
	path := binding.Fixture.Path
	if len(binding.Fixture.Query) != 0 {
		query := url.Values{}
		for _, pair := range binding.Fixture.Query {
			query.Add(pair.Key, pair.Value)
		}
		path += "?" + query.Encode()
	}
	headers := append([]phpServiceFixturePair(nil), binding.Fixture.Headers...)
	body := binding.Fixture.Body
	if binding.Endpoint.RequestMedia == assemblyline.ApplicationServiceEndpointMultipart {
		const boundary = "omnidex-verification-boundary"
		for index := range headers {
			if strings.EqualFold(headers[index].Key, "content-type") {
				headers[index].Value = "multipart/form-data; boundary=" + boundary
			}
		}
		body = "--" + boundary + "\r\n" +
			"Content-Disposition: form-data; name=\"fixture\"\r\n\r\n" +
			"value\r\n--" + boundary + "--\r\n"
	}
	return path, headers, body
}

func phpServiceBindingsHaveHTML(bindings []phpServiceFeatureBinding) bool {
	for _, binding := range bindings {
		if binding.HasEndpoint &&
			binding.Endpoint.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
			return true
		}
	}
	return false
}

func indentPHPSource(source, prefix string) string {
	lines := strings.Split(strings.TrimSuffix(source, "\n"), "\n")
	return prefix + strings.Join(lines, "\n"+prefix) + "\n"
}

func phpServiceHTTPVerifierRuntime() string {
	return `function performHttpRequest(string $method, string $path, array $headers, string $body): array
{
    if (!str_starts_with($path, '/') || str_contains($path, "\r") || str_contains($path, "\n")) {
        throw new InvalidArgumentException('HTTP verification path is invalid.');
    }
    for ($attempt = 0; $attempt < 50; $attempt++) {
        $errorNumber = 0;
        $errorText = '';
        $socket = @fsockopen('nginx', 80, $errorNumber, $errorText, 1.0);
        if (!is_resource($socket)) {
            usleep(100000);
            continue;
        }
        stream_set_timeout($socket, 2);
        $request = $method . ' ' . $path . " HTTP/1.0\r\nHost: nginx\r\nConnection: close\r\n";
        foreach ($headers as $name => $value) {
            if (!is_string($name) || !is_string($value) ||
                preg_match('/^[a-z0-9-]+$/i', $name) !== 1 || str_contains($value, "\r") || str_contains($value, "\n")) {
                fclose($socket);
                throw new InvalidArgumentException('HTTP verification header is invalid.');
            }
            $request .= $name . ': ' . $value . "\r\n";
        }
        if ($body !== '') {
            $request .= 'Content-Length: ' . strlen($body) . "\r\n";
        }
        $request .= "\r\n" . $body;
        if (fwrite($socket, $request) !== strlen($request)) {
            fclose($socket);
            throw new RuntimeException('HTTP verification request was not fully written.');
        }
        $response = '';
        while (!feof($socket)) {
            $chunk = fread($socket, 8192);
            if (!is_string($chunk)) {
                fclose($socket);
                throw new RuntimeException('HTTP verification response could not be read.');
            }
            $response .= $chunk;
            if (strlen($response) > 2097152) {
                fclose($socket);
                throw new RuntimeException('HTTP verification response exceeded two MiB.');
            }
        }
        $metadata = stream_get_meta_data($socket);
        fclose($socket);
        if (($metadata['timed_out'] ?? false) === true) {
            throw new RuntimeException('HTTP verification response timed out.');
        }
        $parsed = parseHttpResponse($response);
        if (($parsed['status'] === 502 || $parsed['status'] === 503) && $attempt < 49) {
            usleep(100000);
            continue;
        }
        return $parsed;
    }
    throw new RuntimeException('NGINX did not accept a connection.');
}

function parseHttpResponse(string $response): array
{
    $separator = strpos($response, "\r\n\r\n");
    if ($separator === false) {
        throw new RuntimeException('HTTP response has no header boundary.');
    }
    $head = substr($response, 0, $separator);
    $body = substr($response, $separator + 4);
    $lines = explode("\r\n", $head);
    if ($lines === [] || preg_match('/^HTTP\/[0-9.]+ ([0-9]{3})(?: |$)/', $lines[0], $match) !== 1) {
        throw new RuntimeException('HTTP response has no valid status line.');
    }
    $headers = [];
    foreach (array_slice($lines, 1) as $line) {
        $colon = strpos($line, ':');
        if ($colon === false) {
            throw new RuntimeException('HTTP response contains an invalid header.');
        }
        $headers[strtolower(trim(substr($line, 0, $colon)))] = trim(substr($line, $colon + 1));
    }
    return ['status' => (int) $match[1], 'headers' => $headers, 'body' => $body];
}

function verifyHttpResponse(array $response, int $status, string $media): void
{
    if ($response['status'] !== $status) {
        throw new RuntimeException('HTTP response status differs from its typed endpoint contract.');
    }
    $contentType = strtolower(trim(explode(';', $response['headers']['content-type'] ?? '')[0]));
    if ($media === 'none') {
        if ($response['body'] !== '') {
            throw new RuntimeException('No-content HTTP response contains a body.');
        }
        return;
    }
    if ($contentType !== $media) {
        throw new RuntimeException('HTTP response media differs from its typed endpoint contract.');
    }
    if ($media === 'application/json') {
        $decoded = json_decode($response['body'], true, 512, JSON_THROW_ON_ERROR);
        if (!is_array($decoded) || !array_key_exists('output', $decoded) ||
            !array_key_exists('error', $decoded) || !array_key_exists('state', $decoded) || $decoded['error'] !== '') {
            throw new RuntimeException('JSON response lost the TaskResult boundary.');
        }
    }
    if ($media === 'application/xml' &&
        (!str_starts_with($response['body'], '<result>') || !str_ends_with($response['body'], '</result>'))) {
        throw new RuntimeException('XML response lost the result document boundary.');
    }
    if ($media === 'text/html') {
        $lower = strtolower($response['body']);
        if (!str_starts_with($lower, '<!doctype html>') || !str_contains($lower, '<main') ||
            !str_contains($lower, '<h1') || str_contains($lower, '<script') || str_contains($lower, 'javascript:')) {
            throw new RuntimeException('HTML response is not one bounded server-rendered document.');
        }
    }
}

function verifyStylesheetResponse(array $response): void
{
    $contentType = strtolower(trim(explode(';', $response['headers']['content-type'] ?? '')[0]));
    if ($response['status'] !== 200 || $contentType !== 'text/css' || trim($response['body']) === '') {
        throw new RuntimeException('Compiled Tailwind stylesheet is not served through NGINX.');
    }
}`
}
