package worker

import (
	"reflect"
	"testing"
)

func TestServiceDeploymentDescriptorsAreTechnicalAndExact(t *testing.T) {
	t.Parallel()
	for _, descriptor := range []*directCodingDeploymentDescriptor{
		genericPHPDeploymentDescriptor(), laravelDeploymentDescriptor(),
	} {
		if err := descriptor.validate(); err != nil {
			t.Fatal(err)
		}
		if descriptor.ReadinessPath != directCodingDeploymentReadinessPath ||
			descriptor.GatewayService != "nginx" || descriptor.GatewayContainerPort != 80 {
			t.Fatalf("descriptor=%+v", descriptor)
		}
	}
}

func TestDeploymentDescriptorProjectsOnlyRequiredEnvironmentAndServices(t *testing.T) {
	t.Parallel()
	requestLocal := laravelFixtureProgram(t, laravelWeatherFixtureInput())
	descriptor := laravelDeploymentDescriptor()
	settings := DeploymentSettings{BindAddress: "127.0.0.1"}
	secrets := map[string]string{"APP_KEY": "base64:key", "DATABASE_PASSWORD": "database"}
	environment, err := descriptor.environment(requestLocal, settings, 0, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := environment["DATABASE_PASSWORD"]; exists ||
		environment["APP_KEY"] == "" || environment["HOST_HTTP_PORT"] != "0" {
		t.Fatalf("request-local environment=%+v", environment)
	}
	services, err := descriptor.expectedServices(requestLocal)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(services, []string{"app", "nginx"}) {
		t.Fatalf("request-local services=%v", services)
	}

	durable := laravelFixtureProgram(t, laravelCheckoutFixtureInput())
	environment, err = descriptor.environment(durable, settings, 49173, secrets)
	if err != nil {
		t.Fatal(err)
	}
	services, err = descriptor.expectedServices(durable)
	if err != nil {
		t.Fatal(err)
	}
	if environment["DATABASE_PASSWORD"] == "" || environment["HOST_HTTP_PORT"] != "49173" ||
		!reflect.DeepEqual(services, []string{"app", "db", "nginx"}) {
		t.Fatalf("durable environment=%+v services=%v", environment, services)
	}
}

func TestDeploymentProjectIdentityIsCodeDerived(t *testing.T) {
	t.Parallel()
	project, err := directCodingStableDeploymentProjectName(41)
	if err != nil {
		t.Fatal(err)
	}
	args, err := directCodingDeploymentComposeArgs(
		project, "up", "--detach", "--wait", "--remove-orphans",
	)
	if err != nil {
		t.Fatal(err)
	}
	if project != "omnidex-project-41" || len(args) != 9 || args[2] != project ||
		args[3] != "--file" || args[4] != directCodingDeploymentComposePath {
		t.Fatalf("project=%q args=%v", project, args)
	}
}
