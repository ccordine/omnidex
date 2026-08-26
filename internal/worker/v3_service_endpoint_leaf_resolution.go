package worker

import (
	"github.com/gryph/omnidex/internal/assemblyline"
)

func resolveDirectCodingServiceEndpointContractLeaves(
	runtime typedWorkerRuntime,
	models directCodingServiceEndpointLeafModels,
	authority assemblyline.ApplicationServiceEndpointTaskAuthority,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationServiceEndpointContract, error) {
	exposureInput := assemblyline.ApplicationServiceEndpointExposureInput{Task: authority}
	exposureJob, err := assemblyline.NewApplicationServiceEndpointExposureJob(exposureInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	exposure, err := runDirectCodingSemanticCall[assemblyline.ApplicationServiceEndpointExposureResult](
		runtime, models.Exposure, string(assemblyline.WorkApplicationServiceEndpointExposure), exposureJob, identities,
		func(value assemblyline.ApplicationServiceEndpointExposureResult) error {
			return value.ValidateFor(exposureInput)
		},
	)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}

	methodInput := assemblyline.ApplicationServiceEndpointMethodInput{Task: authority}
	methodJob, err := assemblyline.NewApplicationServiceEndpointMethodJob(methodInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	method, err := runDirectCodingSemanticCall[assemblyline.ApplicationServiceEndpointMethodResult](
		runtime, models.Method, string(assemblyline.WorkApplicationServiceEndpointMethod), methodJob, identities,
		func(value assemblyline.ApplicationServiceEndpointMethodResult) error {
			return value.ValidateFor(methodInput)
		},
	)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}

	routeInput := assemblyline.ApplicationServiceEndpointRouteTemplateInput{Task: authority}
	routeJob, err := assemblyline.NewApplicationServiceEndpointRouteTemplateJob(routeInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	route, err := runDirectCodingSemanticCall[assemblyline.ApplicationServiceEndpointRouteTemplateResult](
		runtime, models.Route, string(assemblyline.WorkApplicationServiceEndpointRouteTemplate), routeJob, identities,
		func(value assemblyline.ApplicationServiceEndpointRouteTemplateResult) error {
			return value.ValidateFor(routeInput)
		},
	)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}

	requestInput := assemblyline.ApplicationServiceEndpointRequestMediaInput{
		Task: authority, Method: method.Method,
	}
	requestCandidates, err := assemblyline.ApplicationServiceEndpointRequestMediaCandidates(requestInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	requestMedia := assemblyline.ApplicationServiceEndpointRequestMediaResult{
		Schema: assemblyline.ApplicationServiceEndpointRequestMediaSchemaV1,
	}
	if len(requestCandidates) == 1 {
		requestMedia.RequestMedia = requestCandidates[0]
	} else {
		requestJob, jobErr := assemblyline.NewApplicationServiceEndpointRequestMediaJob(requestInput)
		if jobErr != nil {
			return assemblyline.ApplicationServiceEndpointContract{}, jobErr
		}
		requestMedia, err = runDirectCodingSemanticCall[assemblyline.ApplicationServiceEndpointRequestMediaResult](
			runtime, models.RequestMedia, string(assemblyline.WorkApplicationServiceEndpointRequestMedia), requestJob, identities,
			func(value assemblyline.ApplicationServiceEndpointRequestMediaResult) error {
				return value.ValidateFor(requestInput)
			},
		)
		if err != nil {
			return assemblyline.ApplicationServiceEndpointContract{}, err
		}
	}

	responseInput := assemblyline.ApplicationServiceEndpointResponseMediaInput{Task: authority}
	responseJob, err := assemblyline.NewApplicationServiceEndpointResponseMediaJob(responseInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	responseMedia, err := runDirectCodingSemanticCall[assemblyline.ApplicationServiceEndpointResponseMediaResult](
		runtime, models.ResponseMedia, string(assemblyline.WorkApplicationServiceEndpointResponseMedia), responseJob, identities,
		func(value assemblyline.ApplicationServiceEndpointResponseMediaResult) error {
			return value.ValidateFor(responseInput)
		},
	)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}

	statusInput := assemblyline.ApplicationServiceEndpointSuccessStatusInput{
		Task: authority, Method: method.Method,
		RequestMedia: requestMedia.RequestMedia, ResponseMedia: responseMedia.ResponseMedia,
	}
	statusCandidates, err := assemblyline.ApplicationServiceEndpointSuccessStatusCandidates(statusInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	status := assemblyline.ApplicationServiceEndpointSuccessStatusResult{
		Schema: assemblyline.ApplicationServiceEndpointSuccessStatusSchemaV1,
	}
	if len(statusCandidates) == 1 {
		status.SuccessStatus = statusCandidates[0]
	} else {
		statusJob, jobErr := assemblyline.NewApplicationServiceEndpointSuccessStatusJob(statusInput)
		if jobErr != nil {
			return assemblyline.ApplicationServiceEndpointContract{}, jobErr
		}
		status, err = runDirectCodingSemanticCall[assemblyline.ApplicationServiceEndpointSuccessStatusResult](
			runtime, models.SuccessStatus, string(assemblyline.WorkApplicationServiceEndpointSuccessStatus), statusJob, identities,
			func(value assemblyline.ApplicationServiceEndpointSuccessStatusResult) error {
				return value.ValidateFor(statusInput)
			},
		)
		if err != nil {
			return assemblyline.ApplicationServiceEndpointContract{}, err
		}
	}

	return assemblyline.ComposeApplicationServiceEndpointContract(
		authority, exposure, method, route, requestMedia, responseMedia, status,
	)
}
