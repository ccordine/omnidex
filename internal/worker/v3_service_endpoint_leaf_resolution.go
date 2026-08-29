package worker

import "github.com/gryph/omnidex/internal/assemblyline"

type directCodingServiceEndpointLeafModels struct {
	Exposure      string
	Method        string
	Route         string
	RequestMedia  string
	ResponseMedia string
	SuccessStatus string
}

func resolveDirectCodingServiceEndpointContractLeaves(
	runtime typedWorkerRuntime,
	models directCodingServiceEndpointLeafModels,
	authority assemblyline.ApplicationServiceEndpointTaskAuthority,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationServiceEndpointContract, error) {
	exposureInput := assemblyline.ApplicationServiceEndpointExposureInput{Authority: authority}
	exposureJob, err := assemblyline.NewApplicationServiceEndpointExposureJob(exposureInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	exposure, err := runDirectCodingSemanticLeafCall(
		runtime, models.Exposure, string(assemblyline.WorkApplicationServiceEndpointExposure), exposureJob, identities,
		func(raw string) (assemblyline.ApplicationServiceEndpointExposureResult, error) {
			return assemblyline.DecodeApplicationServiceEndpointExposureResult(exposureInput, raw)
		},
		func(value assemblyline.ApplicationServiceEndpointExposureResult) error {
			return value.ValidateFor(exposureInput)
		},
	)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}

	methodInput := assemblyline.ApplicationServiceEndpointMethodInput{Authority: authority}
	methodJob, err := assemblyline.NewApplicationServiceEndpointMethodJob(methodInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	method, err := runDirectCodingSemanticLeafCall(
		runtime, models.Method, string(assemblyline.WorkApplicationServiceEndpointMethod), methodJob, identities,
		func(raw string) (assemblyline.ApplicationServiceEndpointMethodResult, error) {
			return assemblyline.DecodeApplicationServiceEndpointMethodResult(methodInput, raw)
		},
		func(value assemblyline.ApplicationServiceEndpointMethodResult) error {
			return value.ValidateFor(methodInput)
		},
	)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}

	routeInput := assemblyline.ApplicationServiceEndpointRouteTemplateInput{Authority: authority}
	routeJob, err := assemblyline.NewApplicationServiceEndpointRouteTemplateJob(routeInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	route, err := runDirectCodingSemanticLeafCall(
		runtime, models.Route, string(assemblyline.WorkApplicationServiceEndpointRouteTemplate), routeJob, identities,
		func(raw string) (assemblyline.ApplicationServiceEndpointRouteTemplateResult, error) {
			return assemblyline.DecodeApplicationServiceEndpointRouteTemplateResult(routeInput, raw)
		},
		func(value assemblyline.ApplicationServiceEndpointRouteTemplateResult) error {
			return value.ValidateFor(routeInput)
		},
	)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}

	requestInput := assemblyline.ApplicationServiceEndpointRequestMediaInput{
		Authority: authority, Method: method.Method,
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
		requestMedia, err = runDirectCodingSemanticLeafCall(
			runtime, models.RequestMedia, string(assemblyline.WorkApplicationServiceEndpointRequestMedia), requestJob, identities,
			func(raw string) (assemblyline.ApplicationServiceEndpointRequestMediaResult, error) {
				return assemblyline.DecodeApplicationServiceEndpointRequestMediaResult(requestInput, raw)
			},
			func(value assemblyline.ApplicationServiceEndpointRequestMediaResult) error {
				return value.ValidateFor(requestInput)
			},
		)
		if err != nil {
			return assemblyline.ApplicationServiceEndpointContract{}, err
		}
	}

	responseInput := assemblyline.ApplicationServiceEndpointResponseMediaInput{
		Authority: authority, Method: method.Method,
	}
	responseJob, err := assemblyline.NewApplicationServiceEndpointResponseMediaJob(responseInput)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}
	responseMedia, err := runDirectCodingSemanticLeafCall(
		runtime, models.ResponseMedia, string(assemblyline.WorkApplicationServiceEndpointResponseMedia), responseJob, identities,
		func(raw string) (assemblyline.ApplicationServiceEndpointResponseMediaResult, error) {
			return assemblyline.DecodeApplicationServiceEndpointResponseMediaResult(responseInput, raw)
		},
		func(value assemblyline.ApplicationServiceEndpointResponseMediaResult) error {
			return value.ValidateFor(responseInput)
		},
	)
	if err != nil {
		return assemblyline.ApplicationServiceEndpointContract{}, err
	}

	statusInput := assemblyline.ApplicationServiceEndpointSuccessStatusInput{
		Authority: authority, Method: method.Method,
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
		status, err = runDirectCodingSemanticLeafCall(
			runtime, models.SuccessStatus, string(assemblyline.WorkApplicationServiceEndpointSuccessStatus), statusJob, identities,
			func(raw string) (assemblyline.ApplicationServiceEndpointSuccessStatusResult, error) {
				return assemblyline.DecodeApplicationServiceEndpointSuccessStatusResult(statusInput, raw)
			},
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
