package version

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeScriptsVerifyActualImageUserAndHealth(t *testing.T) {
	t.Parallel()
	library, err := filepath.Abs(filepath.Join("..", "..", "scripts", "update-runtime-lib.sh"))
	if err != nil {
		t.Fatal(err)
	}
	imageID := "sha256:" + strings.Repeat("a", 64)
	otherImageID := "sha256:" + strings.Repeat("b", 64)
	for _, test := range []struct {
		name, action, image, user, health, response, wantErr string
	}{
		{"built image user", "image", imageID, "1000:1000", "healthy", "ok", ""},
		{"wrong built image user", "image", imageID, "0:0", "healthy", "ok", "runtime user"},
		{"running service", "running", imageID, "1000:1000", "healthy", "ok", ""},
		{"wrong running image", "running", otherImageID, "1000:1000", "healthy", "ok", "is running image"},
		{"wrong running user", "running", imageID, "0:0", "healthy", "ok", "is running as"},
		{"unhealthy container", "running", imageID, "1000:1000", "unhealthy", "ok", "health is unhealthy"},
		{"bad typed health", "running", imageID, "1000:1000", "healthy", "degraded", "health reports degraded"},
		{"failed health command", "running", imageID, "1000:1000", "healthy", "command_failed", "typed health verification failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := exec.Command("bash", "-eu", "-c", runtimeHealthScriptFixture,
				"runtime-test", library, test.action, imageID, test.image,
				test.user, test.health, test.response, t.TempDir()).CombinedOutput()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("runtime verification failed: %s (%v)", output, err)
				}
			} else if err == nil || !strings.Contains(string(output), test.wantErr) {
				t.Fatalf("error = %v, output = %s, want %q", err, output, test.wantErr)
			}
		})
	}
}

// These mocks exercise the checked-in deployment checks without changing a
// Docker installation. Every unexpected command fails, including old commit gates.
const runtimeHealthScriptFixture = `
source "$1"
die() { printf '%s\n' "$*" >&2; exit 1; }
log() { printf '%s\n' "$*"; }
action="$2"
expected_image="$3"
observed_image="$4"
observed_user="$5"
observed_health="$6"
observed_response="$7"
repo="$8"
test_container=0123456789abcdef
COMPOSE_PROJECT=fixture
HOST_UID=1000
HOST_GID=1000
compose_docker() {
  [[ "$*" == '-p fixture ps -q core' ]] || die "unexpected compose command: $*"
  printf '%s\n' "$test_container"
}
context_docker() {
  case "$1" in
    image)
      [[ "$#" == 5 && "$2" == inspect && "$3" == --format &&
         "$4" == '{{.Config.User}}' && "$5" == "$expected_image" ]] ||
        die "unexpected image command: $*"
      printf '%s\n' "$observed_user"
      ;;
    inspect)
      [[ "$#" == 6 && "$2" == --type && "$3" == container &&
         "$4" == --format && "$6" == "$test_container" ]] ||
        die "unexpected container command: $*"
      case "$5" in
        '{{.Image}}') printf '%s\n' "$observed_image" ;;
        '{{.Config.User}}') printf '%s\n' "$observed_user" ;;
        '{{if .State.Health}}{{.State.Health.Status}}{{end}}') printf '%s\n' "$observed_health" ;;
        *) die "unexpected container field: $5" ;;
      esac
      ;;
    exec)
      [[ "$*" == "exec $test_container /usr/local/bin/omnidex health" ]] ||
        die "unexpected health command: $*"
      [[ "$observed_response" != command_failed ]] || return 1
      printf '%s\n' "$observed_response"
      ;;
    *) die "unexpected Docker command: $*" ;;
  esac
}
case "$action" in
  image) compose_require_image_user "$expected_image" 1000:1000 ;;
  running) compose_require_running_image "$repo" 'docker compose' '' core "$expected_image" 1000:1000 ;;
  *) die "unknown test action" ;;
esac
`
