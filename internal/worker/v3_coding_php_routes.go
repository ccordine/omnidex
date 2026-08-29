package worker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var phpServiceRouteParameter = regexp.MustCompile(`\{([a-z][a-z0-9]*(?:_[a-z0-9]+)*)\}`)

func phpServiceRouteBlocks(bindings []phpServiceFeatureBinding) ([]assemblyline.SourceBlock, error) {
	blocks := make([]assemblyline.SourceBlock, 0, len(bindings))
	for _, binding := range bindings {
		if !binding.HasEndpoint {
			continue
		}
		signature, source, err := phpServiceRouteFunction(binding)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: binding.RouteBlockID, Static: source, API: signature,
			TaskID: binding.TaskID, Role: assemblyline.SourceBlockTaskSupport,
		})
	}
	return blocks, nil
}

func phpServiceRouteFunction(binding phpServiceFeatureBinding) (string, string, error) {
	if !binding.HasEndpoint || binding.RouteBlockID == "" || binding.RouteName == "" {
		return "", "", fmt.Errorf("PHP HTTP route helper requires one endpoint binding")
	}
	if err := assemblyline.ValidateApplicationServiceRouteTemplate(binding.Endpoint.RouteTemplate); err != nil {
		return "", "", err
	}
	parameters := phpServiceRouteParameters(binding.Endpoint.RouteTemplate)
	arguments := make([]string, len(parameters))
	for index, parameter := range parameters {
		arguments[index] = "string $" + parameter
	}
	signature := fmt.Sprintf("function %s(%s): RuntimeRoute", binding.RouteName, strings.Join(arguments, ", "))
	var source strings.Builder
	source.WriteString(signature)
	source.WriteString("\n{\n")
	for _, parameter := range parameters {
		source.WriteString(fmt.Sprintf(
			"    if ($%s === '') { throw new InvalidArgumentException(%s); }\n",
			parameter, phpSingleQuoted("route parameter "+parameter+" is required"),
		))
	}
	segments := strings.Split(strings.Trim(binding.Endpoint.RouteTemplate, "/"), "/")
	expressions := make([]string, 0, len(segments))
	if binding.Endpoint.RouteTemplate == "/" {
		expressions = append(expressions, phpSingleQuoted(""))
	} else {
		for _, segment := range segments {
			match := phpServiceRouteParameter.FindStringSubmatch(segment)
			if match == nil {
				expressions = append(expressions, phpSingleQuoted(segment))
				continue
			}
			expressions = append(expressions, "rawurlencode($"+match[1]+")")
		}
	}
	source.WriteString("    return new RuntimeRoute('/' . ")
	source.WriteString(strings.Join(expressions, " . '/' . "))
	source.WriteString(", ")
	source.WriteString(phpSingleQuoted(string(binding.Endpoint.Method)))
	source.WriteString(");\n}\n")
	return signature, source.String(), nil
}

func phpServiceRouteParameters(route string) []string {
	matches := phpServiceRouteParameter.FindAllStringSubmatch(route, -1)
	parameters := make([]string, len(matches))
	for index, match := range matches {
		parameters[index] = match[1]
	}
	return parameters
}

func phpServiceRendererRouteBindings(
	owner phpServiceFeatureBinding,
	capabilities []directCodingCapabilityBinding,
	byRequirement map[string]phpServiceFeatureBinding,
) ([]phpServiceFeatureBinding, error) {
	routes := make([]phpServiceFeatureBinding, 0, len(capabilities)+1)
	seen := make(map[string]struct{}, len(capabilities)+1)
	add := func(binding phpServiceFeatureBinding) {
		if !binding.HasEndpoint {
			return
		}
		if _, exists := seen[binding.TaskID]; exists {
			return
		}
		seen[binding.TaskID] = struct{}{}
		routes = append(routes, binding)
	}
	add(owner)
	for _, capability := range capabilities {
		binding, exists := byRequirement[capability.RequirementID]
		if !exists {
			return nil, fmt.Errorf(
				"HTML route capability names unknown requirement %s", capability.RequirementID,
			)
		}
		add(binding)
	}
	return routes, nil
}

func phpServiceRendererRouteContract(routes []phpServiceFeatureBinding) []string {
	lines := make([]string, 0, len(routes))
	for _, route := range routes {
		lines = append(lines, fmt.Sprintf(
			"Generate the interaction for %s by calling %s with HTTP %s.",
			route.RequirementQuote, route.RouteName, route.Endpoint.Method,
		))
	}
	return lines
}
