<?php

declare(strict_types=1);

namespace Omnidex\Integration\Http;

final class NativeTransport implements Transport
{
    private const MAX_RESPONSE_BYTES = 16 * 1024 * 1024;

    public function send(
        string $method,
        string $url,
        array $headers,
        ?string $body,
        int $timeoutSeconds,
    ): Response {
        $lines = [];
        foreach ($headers as $name => $value) {
            if (str_contains($name . $value, "\r") || str_contains($name . $value, "\n")) {
                throw new \InvalidArgumentException('HTTP headers must not contain line breaks.');
            }
            $lines[] = $name . ': ' . $value;
        }
        $context = stream_context_create([
            'http' => [
                'method' => $method,
                'header' => implode("\r\n", $lines),
                'content' => $body ?? '',
                'timeout' => $timeoutSeconds,
                'ignore_errors' => true,
                'follow_location' => 0,
                'max_redirects' => 0,
            ],
        ]);
        $stream = @fopen($url, 'rb', false, $context);
        if ($stream === false) {
            throw new \RuntimeException('Execute Omnidex integration request failed.');
        }
        try {
            $payload = stream_get_contents($stream, self::MAX_RESPONSE_BYTES + 1);
            if ($payload === false) {
                throw new \RuntimeException('Read Omnidex integration response failed.');
            }
            if (strlen($payload) > self::MAX_RESPONSE_BYTES) {
                throw new \RuntimeException('Omnidex integration response exceeds 16777216 bytes.');
            }
            $metadata = stream_get_meta_data($stream);
        } finally {
            fclose($stream);
        }
        $wrapper = $metadata['wrapper_data'] ?? null;
        if (!is_array($wrapper) || $wrapper === []) {
            throw new \RuntimeException('Omnidex integration response has no HTTP authority.');
        }
        return self::response($wrapper, $payload);
    }

    /** @param list<string> $lines */
    private static function response(array $lines, string $body): Response
    {
        if (preg_match('/^HTTP\/\S+ ([1-5][0-9]{2})(?: |$)/', $lines[0], $match) !== 1) {
            throw new \RuntimeException('Omnidex integration response has an invalid status line.');
        }
        $headers = [];
        foreach (array_slice($lines, 1) as $line) {
            $separator = strpos($line, ':');
            if ($separator === false) {
                continue;
            }
            $name = strtolower(trim(substr($line, 0, $separator)));
            $value = trim(substr($line, $separator + 1));
            if (array_key_exists($name, $headers)) {
                throw new \RuntimeException('Omnidex integration response repeats an HTTP header.');
            }
            $headers[$name] = $value;
        }
        return new Response((int) $match[1], $headers, $body);
    }
}
