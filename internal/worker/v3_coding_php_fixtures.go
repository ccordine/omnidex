package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type phpServiceFixturePair struct {
	Key   string
	Value string
}

type phpServiceInputFixture struct {
	Method          string
	Path            string
	RouteParameters []phpServiceFixturePair
	Query           []phpServiceFixturePair
	Headers         []phpServiceFixturePair
	Post            []phpServiceFixturePair
	Body            string
	Payload         string
}

func phpServiceTaskInputFixture(
	requirement assemblyline.ApplicationServiceEndpointRequirement,
	contract assemblyline.ApplicationServiceEndpointContract,
) (phpServiceInputFixture, error) {
	if requirement == assemblyline.ApplicationServiceSupportOnly {
		return phpServiceInputFixture{
			Method: "LOCAL", Path: "/",
			Query:   []phpServiceFixturePair{{Key: "fixture", Value: "value"}},
			Payload: "['fixture' => 'value']",
		}, nil
	}
	if requirement != assemblyline.ApplicationServiceEndpointRequired {
		return phpServiceInputFixture{}, fmt.Errorf(
			"PHP task fixture has unsupported endpoint decision %q", requirement,
		)
	}
	segments := strings.Split(strings.Trim(contract.RouteTemplate, "/"), "/")
	parameters := make([]phpServiceFixturePair, 0)
	for index, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			parameters = append(parameters, phpServiceFixturePair{Key: name, Value: "1"})
			segments[index] = "1"
		}
	}
	fixturePath := contract.RouteTemplate
	if contract.RouteTemplate != "/" {
		fixturePath = "/" + strings.Join(segments, "/")
	}
	fixture := phpServiceInputFixture{
		Method: string(contract.Method), Path: fixturePath, RouteParameters: parameters,
		Query:   []phpServiceFixturePair{{Key: "fixture", Value: "value"}},
		Headers: []phpServiceFixturePair{{Key: "accept", Value: string(contract.ResponseMedia)}},
		Payload: "null",
	}
	if contract.RequestMedia != assemblyline.ApplicationServiceEndpointMediaNone {
		fixture.Headers = append(fixture.Headers, phpServiceFixturePair{
			Key: "content-type", Value: string(contract.RequestMedia),
		})
	}
	switch contract.RequestMedia {
	case assemblyline.ApplicationServiceEndpointMediaNone:
	case assemblyline.ApplicationServiceEndpointJSON:
		fixture.Body = `{"fixture":"value"}`
		fixture.Payload = "['fixture' => 'value']"
	case assemblyline.ApplicationServiceEndpointForm:
		fixture.Body = "fixture=value"
		fixture.Post = []phpServiceFixturePair{{Key: "fixture", Value: "value"}}
		fixture.Payload = "['fixture' => 'value']"
	case assemblyline.ApplicationServiceEndpointMultipart:
		fixture.Post = []phpServiceFixturePair{{Key: "fixture", Value: "value"}}
		fixture.Payload = "['fields' => ['fixture' => 'value'], 'files' => []]"
	case assemblyline.ApplicationServiceEndpointXML:
		fixture.Body = "<fixture>value</fixture>"
		fixture.Payload = phpSingleQuoted(fixture.Body)
	case assemblyline.ApplicationServiceEndpointText:
		fixture.Body = "value"
		fixture.Payload = phpSingleQuoted(fixture.Body)
	default:
		return phpServiceInputFixture{}, fmt.Errorf(
			"PHP task fixture has unsupported request media %q", contract.RequestMedia,
		)
	}
	return fixture, nil
}

func phpServiceTaskInputFixtureSource(
	binding phpServiceFeatureBinding,
) string {
	return fmt.Sprintf(`function %s(): TaskInput
{
    return %s;
}`,
		binding.FixtureName, phpServiceTaskInputExpression(binding.Fixture),
	)
}

func phpServiceTaskInputExpression(fixture phpServiceInputFixture) string {
	return fmt.Sprintf(
		"new TaskInput(%s, %s, %s, %s, %s, %s, %s)",
		phpSingleQuoted(fixture.Method), phpSingleQuoted(fixture.Path),
		phpServiceFixtureArray(fixture.RouteParameters), phpServiceFixtureArray(fixture.Query),
		phpServiceFixtureArray(fixture.Headers), phpSingleQuoted(fixture.Body), fixture.Payload,
	)
}

func phpServiceHTTPRequestExpression(fixture phpServiceInputFixture) string {
	return fmt.Sprintf(
		"new HttpRequest(%s, %s, %s, %s, %s, [], %s)",
		phpSingleQuoted(fixture.Method), phpSingleQuoted(fixture.Path),
		phpServiceFixtureArray(fixture.Query), phpServiceFixtureArray(fixture.Headers),
		phpServiceFixtureArray(fixture.Post), phpSingleQuoted(fixture.Body),
	)
}

func phpServiceFixtureArray(pairs []phpServiceFixturePair) string {
	if len(pairs) == 0 {
		return "[]"
	}
	values := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		values = append(values, phpSingleQuoted(pair.Key)+" => "+phpSingleQuoted(pair.Value))
	}
	return "[" + strings.Join(values, ", ") + "]"
}

func phpServiceHTTPSmokeSource(bindings []phpServiceFeatureBinding, stateful bool) string {
	var source strings.Builder
	source.WriteString("try {\n")
	for _, binding := range bindings {
		if !binding.HasEndpoint {
			continue
		}
		if stateful {
			source.WriteString("    " + phpServiceStateResetFunctionName + "();\n")
		}
		response := "$httpResponse" + binding.FeatureNumber
		source.WriteString(fmt.Sprintf(
			"    %s = dispatchApplicationHttp(%s);\n",
			response, phpServiceHTTPRequestExpression(binding.Fixture),
		))
		source.WriteString(fmt.Sprintf(
			"    if (%s->status !== %d || %s->media !== %s) {\n",
			response, binding.Endpoint.SuccessStatus, response,
			phpSingleQuoted(string(binding.Endpoint.ResponseMedia)),
		))
		source.WriteString(fmt.Sprintf(
			"        throw new RuntimeException(%s);\n",
			phpSingleQuoted("HTTP contract failed for "+binding.FeatureName),
		))
		source.WriteString("    }\n")
		if binding.Endpoint.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
			source.WriteString(fmt.Sprintf(
				"    if (!str_starts_with(%s->body, '<!doctype html>')) {\n", response,
			))
			source.WriteString("        throw new RuntimeException('HTML response was not rendered by the server.');\n")
			source.WriteString("    }\n")
		}
		if stateful {
			source.WriteString("    " + phpServiceStateResetFunctionName + "();\n")
		}
		methodFixture := binding.Fixture
		methodFixture.Method = "OPTIONS"
		source.WriteString(fmt.Sprintf(
			"    $methodResponse%s = dispatchApplicationHttp(%s);\n",
			binding.FeatureNumber, phpServiceHTTPRequestExpression(methodFixture),
		))
		source.WriteString(fmt.Sprintf(
			"    if ($methodResponse%s->status !== 405 || $methodResponse%s->media !== 'text/plain') {\n",
			binding.FeatureNumber, binding.FeatureNumber,
		))
		source.WriteString("        throw new RuntimeException('HTTP method contract was not enforced.');\n")
		source.WriteString("    }\n")
	}
	unmatched := phpServiceUnmatchedRoute(bindings)
	source.WriteString(fmt.Sprintf(
		"    $routeResponse = dispatchApplicationHttp(new HttpRequest('GET', %s, [], [], [], [], ''));\n",
		phpSingleQuoted(unmatched),
	))
	source.WriteString("    if ($routeResponse->status !== 404 || $routeResponse->media !== 'text/plain') {\n")
	source.WriteString("        throw new RuntimeException('HTTP route contract was not enforced.');\n")
	source.WriteString("    }\n")
	source.WriteString("} catch (Throwable $failure) {\n")
	source.WriteString("    $failures[] = 'HTTP contracts: ' . $failure->getMessage();\n")
	source.WriteString("}\n")
	return source.String()
}

func phpServiceUnmatchedRoute(bindings []phpServiceFeatureBinding) string {
	maximum := 0
	for _, binding := range bindings {
		if !binding.HasEndpoint || binding.Endpoint.RouteTemplate == "/" {
			continue
		}
		segments := len(strings.Split(strings.Trim(binding.Endpoint.RouteTemplate, "/"), "/"))
		if segments > maximum {
			maximum = segments
		}
	}
	segments := make([]string, maximum+1)
	for index := range segments {
		segments[index] = "verification"
	}
	return "/" + strings.Join(segments, "/")
}
