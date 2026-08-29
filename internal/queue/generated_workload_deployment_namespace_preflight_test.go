package queue

import (
	"reflect"
	"strings"
	"testing"
)

func TestGeneratedDeploymentNamespacePreflightBindsVacantUnrelatedProjects(t *testing.T) {
	t.Parallel()
	for _, project := range []string{"omnidex-project-17", "fixture-document-catalog"} {
		project := project
		t.Run(project, func(t *testing.T) {
			t.Parallel()
			proof, encoded, err := BindGeneratedWorkloadDeploymentNamespacePreflight(
				GeneratedWorkloadDeploymentNamespacePreflight{
					Schema:         GeneratedWorkloadDeploymentNamespacePreflightV1,
					ComposeProject: project,
					ContainerIDs:   []string{},
					NetworkIDs:     []string{},
					VolumeNames:    []string{},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if proof.SHA256 != generatedDeploymentSHA(encoded) ||
				!GeneratedWorkloadDeploymentNamespaceVacant(proof) {
				t.Fatalf("proof=%+v encoded=%s", proof, encoded)
			}
			second, secondEncoded, err := BindGeneratedWorkloadDeploymentNamespacePreflight(proof)
			if err != nil || !reflect.DeepEqual(second, proof) || secondEncoded != encoded {
				t.Fatalf("rebound=%+v encoded=%s err=%v", second, secondEncoded, err)
			}
		})
	}
}

func TestGeneratedDeploymentNamespacePreflightRejectsNonCanonicalAuthority(t *testing.T) {
	t.Parallel()
	valid := GeneratedWorkloadDeploymentNamespacePreflight{
		Schema:         GeneratedWorkloadDeploymentNamespacePreflightV1,
		ComposeProject: "omnidex-project-19",
		ContainerIDs:   []string{},
		NetworkIDs:     []string{},
		VolumeNames:    []string{},
	}
	for name, mutate := range map[string]func(*GeneratedWorkloadDeploymentNamespacePreflight){
		"wrong schema": func(proof *GeneratedWorkloadDeploymentNamespacePreflight) {
			proof.Schema = "unsupported"
		},
		"invalid project": func(proof *GeneratedWorkloadDeploymentNamespacePreflight) {
			proof.ComposeProject = "Invalid Project"
		},
		"nil resource set": func(proof *GeneratedWorkloadDeploymentNamespacePreflight) {
			proof.NetworkIDs = nil
		},
		"unsorted containers": func(proof *GeneratedWorkloadDeploymentNamespacePreflight) {
			proof.ContainerIDs = []string{strings.Repeat("b", 64), strings.Repeat("a", 64)}
		},
		"duplicate networks": func(proof *GeneratedWorkloadDeploymentNamespacePreflight) {
			proof.NetworkIDs = []string{strings.Repeat("c", 64), strings.Repeat("c", 64)}
		},
		"invalid volume": func(proof *GeneratedWorkloadDeploymentNamespacePreflight) {
			proof.VolumeNames = []string{"volume/name"}
		},
		"forged digest": func(proof *GeneratedWorkloadDeploymentNamespacePreflight) {
			proof.SHA256 = strings.Repeat("f", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			proof := valid
			mutate(&proof)
			if _, _, err := BindGeneratedWorkloadDeploymentNamespacePreflight(proof); err == nil {
				t.Fatalf("noncanonical proof was accepted: %+v", proof)
			}
		})
	}
}
