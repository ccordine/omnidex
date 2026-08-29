package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func laravelRouterDocument(
	bindings []phpServiceFeatureBinding,
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
) (assemblyline.SourceDocument, error) {
	routed := make([]phpServiceFeatureBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.HasEndpoint {
			routed = append(routed, binding)
		}
	}
	if len(routed) == 0 {
		return assemblyline.SourceDocument{}, fmt.Errorf("Laravel router requires one endpoint-required task")
	}
	ordered := phpServiceRouteOrder(routed)
	closure, err := phpServiceRouterClosureWithState(
		ordered, requirements, capabilities, byRequirement, state,
	)
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	preamble := []string{
		"<?php", "declare(strict_types=1);", "",
		"use Illuminate\\Http\\Request;",
		"use Illuminate\\Support\\Facades\\Route;", "",
		"require_once __DIR__ . '/../src/Runtime.php';",
	}
	dependencies := []string{"runtime.laravel.http"}
	for _, binding := range closure {
		preamble = append(preamble, fmt.Sprintf(
			"require_once __DIR__ . '/../src/Feature%s.php';", binding.FeatureNumber,
		))
		if len(capabilities[binding.RequirementID]) != 0 {
			dependencies = append(dependencies, binding.SupportBlockID)
		}
		dependencies = append(dependencies, binding.FeatureBlockID)
		if binding.Endpoint.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
			dependencies = append(dependencies, binding.RendererBlockID)
		}
	}
	source, err := laravelRouterSource(
		ordered, requirements, capabilities, byRequirement, state,
	)
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	return assemblyline.SourceDocument{
		ID: "application_laravel_routes", Path: "routes/web.php", AdapterID: phpSourceAdapterID,
		Preamble: strings.Join(preamble, "\n"),
		Blocks: []assemblyline.SourceBlock{{
			ID: "application.laravel.routes", Static: source,
			API:       "register Laravel routes for the accepted service interactions",
			DependsOn: dependencies,
		}},
	}, nil
}

func laravelRouterSource(
	bindings []phpServiceFeatureBinding,
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	byRequirement map[string]phpServiceFeatureBinding,
	state directCodingServiceStatePlan,
) (string, error) {
	if err := validateLaravelReservedEndpointRoutes(bindings); err != nil {
		return "", err
	}
	var source strings.Builder
	source.WriteString(laravelReadinessRouteSource())
	source.WriteString("\n")
	source.WriteString(laravelMethodRejectionRouteSource(directCodingDeploymentReadinessPath))
	source.WriteString("\n\n")
	rejectedOptions := map[string]struct{}{directCodingDeploymentReadinessPath: {}}
	for _, binding := range bindings {
		if _, registered := rejectedOptions[binding.Endpoint.RouteTemplate]; !registered {
			source.WriteString(laravelMethodRejectionRouteSource(binding.Endpoint.RouteTemplate))
			source.WriteString("\n")
			rejectedOptions[binding.Endpoint.RouteTemplate] = struct{}{}
		}
		source.WriteString(fmt.Sprintf(
			"Route::match([%s], %s, static function (Request $request) {\n",
			phpSingleQuoted(string(binding.Endpoint.Method)),
			phpSingleQuoted(binding.Endpoint.RouteTemplate),
		))
		source.WriteString("    try {\n")
		source.WriteString(fmt.Sprintf(
			"        $input = LaravelRuntime::taskInput($request, %s);\n",
			phpSingleQuoted(string(binding.Endpoint.RequestMedia)),
		))
		source.WriteString("        $results = [];\n")
		executionOrder, err := phpServiceEndpointExecutionOrderWithState(
			binding, requirements, capabilities, byRequirement, state,
		)
		if err != nil {
			return "", err
		}
		for _, execution := range executionOrder {
			invocation, invocationErr := phpServiceFeatureInvocationWithState(
				execution, capabilities, byRequirement, state,
			)
			if invocationErr != nil {
				return "", invocationErr
			}
			source.WriteString(indentPHPSource(
				strings.TrimPrefix(invocation, "            "),
				"        ",
			))
		}
		if binding.Endpoint.ResponseMedia == assemblyline.ApplicationServiceEndpointHTML {
			source.WriteString(fmt.Sprintf(
				"        return LaravelRuntime::response($result%s, %s, %d, %s($result%s));\n",
				binding.FeatureNumber, phpSingleQuoted(string(binding.Endpoint.ResponseMedia)),
				binding.Endpoint.SuccessStatus, binding.RendererName, binding.FeatureNumber,
			))
		} else {
			source.WriteString(fmt.Sprintf(
				"        return LaravelRuntime::response($result%s, %s, %d);\n",
				binding.FeatureNumber, phpSingleQuoted(string(binding.Endpoint.ResponseMedia)),
				binding.Endpoint.SuccessStatus,
			))
		}
		source.WriteString("    } catch (Throwable $failure) {\n")
		source.WriteString("        return LaravelRuntime::failure($failure);\n")
		source.WriteString("    }\n")
		source.WriteString("})->withoutMiddleware([Illuminate\\Foundation\\Http\\Middleware\\ValidateCsrfToken::class]);\n\n")
	}
	source.WriteString("Route::fallback(static fn () => response('Endpoint not found.', 404, ['Content-Type' => 'text/plain']));\n")
	return source.String(), nil
}

func validateLaravelReservedEndpointRoutes(bindings []phpServiceFeatureBinding) error {
	for _, binding := range bindings {
		if binding.HasEndpoint {
			if err := validateLaravelReservedEndpointRoute(
				binding.TaskID, binding.Endpoint.RouteTemplate,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateLaravelReservedEndpointRoute(taskID, route string) error {
	if serviceRouteTemplatesOverlap(route, directCodingDeploymentReadinessPath) {
		return fmt.Errorf(
			"Laravel endpoint task %s route %s overlaps reserved readiness route %s",
			taskID, route, directCodingDeploymentReadinessPath,
		)
	}
	return nil
}

func laravelReadinessRouteSource() string {
	return "Route::get(" + phpSingleQuoted(directCodingDeploymentReadinessPath) +
		", static fn () => response('', 204));"
}

func laravelMethodRejectionRouteSource(route string) string {
	return "Route::options(" + phpSingleQuoted(route) +
		", static fn () => response('HTTP method is not allowed for this endpoint.', 405, " +
		"['Content-Type' => 'text/plain']));"
}
