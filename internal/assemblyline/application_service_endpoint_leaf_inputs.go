package assemblyline

import "fmt"

type ApplicationServiceEndpointExposureInput struct {
	Task ApplicationServiceEndpointTaskAuthority `json:"accepted_local_task_authority"`
}

type ApplicationServiceEndpointMethodInput struct {
	Task ApplicationServiceEndpointTaskAuthority `json:"accepted_local_task_authority"`
}

type ApplicationServiceEndpointRouteTemplateInput struct {
	Task ApplicationServiceEndpointTaskAuthority `json:"accepted_local_task_authority"`
}

type ApplicationServiceEndpointRequestMediaInput struct {
	Task   ApplicationServiceEndpointTaskAuthority `json:"accepted_local_task_authority"`
	Method ApplicationServiceEndpointMethod        `json:"accepted_method"`
}

type ApplicationServiceEndpointResponseMediaInput struct {
	Task ApplicationServiceEndpointTaskAuthority `json:"accepted_local_task_authority"`
}

type ApplicationServiceEndpointSuccessStatusInput struct {
	Task          ApplicationServiceEndpointTaskAuthority `json:"accepted_local_task_authority"`
	Method        ApplicationServiceEndpointMethod        `json:"accepted_method"`
	RequestMedia  ApplicationServiceEndpointMedia         `json:"accepted_request_media"`
	ResponseMedia ApplicationServiceEndpointMedia         `json:"accepted_response_media"`
}

func NewApplicationServiceEndpointExposureJob(input ApplicationServiceEndpointExposureInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceEndpointExposure, input, input.validate)
}

func NewApplicationServiceEndpointMethodJob(input ApplicationServiceEndpointMethodInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceEndpointMethod, input, input.validate)
}

func NewApplicationServiceEndpointRouteTemplateJob(input ApplicationServiceEndpointRouteTemplateInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceEndpointRouteTemplate, input, input.validate)
}

func NewApplicationServiceEndpointRequestMediaJob(input ApplicationServiceEndpointRequestMediaInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceEndpointRequestMedia, input, input.validate)
}

func NewApplicationServiceEndpointResponseMediaJob(input ApplicationServiceEndpointResponseMediaInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceEndpointResponseMedia, input, input.validate)
}

func NewApplicationServiceEndpointSuccessStatusJob(input ApplicationServiceEndpointSuccessStatusInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceEndpointSuccessStatus, input, input.validate)
}

func (input ApplicationServiceEndpointExposureInput) validate() error {
	return input.Task.validate()
}

func (input ApplicationServiceEndpointMethodInput) validate() error {
	return input.Task.validate()
}

func (input ApplicationServiceEndpointRouteTemplateInput) validate() error {
	return input.Task.validate()
}

func (input ApplicationServiceEndpointRequestMediaInput) validate() error {
	if err := input.Task.validate(); err != nil {
		return err
	}
	if !validApplicationServiceEndpointMethod(input.Method) {
		return fmt.Errorf("request-media prerequisite method %q is unsupported", input.Method)
	}
	return nil
}

func (input ApplicationServiceEndpointResponseMediaInput) validate() error {
	return input.Task.validate()
}

func (input ApplicationServiceEndpointSuccessStatusInput) validate() error {
	if err := input.Task.validate(); err != nil {
		return err
	}
	if !validApplicationServiceEndpointMethod(input.Method) {
		return fmt.Errorf("success-status prerequisite method %q is unsupported", input.Method)
	}
	if !validApplicationServiceRequestMedia(input.RequestMedia) {
		return fmt.Errorf("success-status prerequisite request media %q is unsupported", input.RequestMedia)
	}
	if !validApplicationServiceResponseMedia(input.ResponseMedia) {
		return fmt.Errorf("success-status prerequisite response media %q is unsupported", input.ResponseMedia)
	}
	return nil
}
