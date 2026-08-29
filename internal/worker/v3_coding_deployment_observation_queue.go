package worker

import "github.com/gryph/omnidex/internal/queue"

func directCodingQueueDeploymentObservation(
	observation directCodingDeploymentObservation,
) queue.GeneratedWorkloadDeploymentObservation {
	services := make([]queue.GeneratedWorkloadDeploymentObservedService, len(observation.Services))
	for index, service := range observation.Services {
		services[index] = queue.GeneratedWorkloadDeploymentObservedService{
			Service: service.Service, ContainerID: service.ContainerID,
			ImageDigest: service.ImageID, RestartPolicy: service.RestartPolicy,
			State: service.State, Health: service.Health,
		}
	}
	return queue.GeneratedWorkloadDeploymentObservation{
		Schema: observation.Schema, Project: observation.Project, Services: services,
		Endpoint: queue.GeneratedWorkloadDeploymentObservedEndpoint{
			Scheme: observation.Endpoint.Scheme, Host: observation.Endpoint.Host,
			Port: observation.Endpoint.Port, Path: observation.Endpoint.Path,
		},
		ServicesSHA256: observation.ServicesSHA256,
		EndpointSHA256: observation.EndpointSHA256, SHA256: observation.SHA256,
	}
}

func directCodingWorkerDeploymentObservation(
	observation queue.GeneratedWorkloadDeploymentObservation,
) directCodingDeploymentObservation {
	services := make([]directCodingObservedService, len(observation.Services))
	for index, service := range observation.Services {
		services[index] = directCodingObservedService{
			Service: service.Service, ContainerID: service.ContainerID,
			ImageID: service.ImageDigest, RestartPolicy: service.RestartPolicy,
			State: service.State, Health: service.Health,
		}
	}
	return directCodingDeploymentObservation{
		Schema: observation.Schema, Project: observation.Project, Services: services,
		Endpoint: directCodingObservedEndpoint{
			Scheme: observation.Endpoint.Scheme, Host: observation.Endpoint.Host,
			Port: observation.Endpoint.Port, Path: observation.Endpoint.Path,
		},
		ServicesSHA256: observation.ServicesSHA256,
		EndpointSHA256: observation.EndpointSHA256, SHA256: observation.SHA256,
	}
}
