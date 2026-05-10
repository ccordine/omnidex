package main

import "testing"

func TestFlagProvided(t *testing.T) {
	args := []string{"--web=on", "--workspace", "off", "--verbose=false"}
	if !flagProvided(args, "web") {
		t.Fatal("expected web flag to be detected")
	}
	if !flagProvided(args, "workspace") {
		t.Fatal("expected workspace flag to be detected")
	}
	if !flagProvided(args, "verbose") {
		t.Fatal("expected verbose flag to be detected")
	}
	if flagProvided(args, "missing") {
		t.Fatal("did not expect missing flag")
	}
}

func TestApplyExecutionProfileArchitectDefaults(t *testing.T) {
	web := "off"
	workspace := "off"
	allowMissing := false
	reasoning := "fast"
	autonomy := "auto"
	approval := "off"
	verify := "off"
	verifyIterations := 1
	verbose := false
	maxChars := 1200
	localShell := false

	architect, err := applyExecutionProfile(
		[]string{"--profile", "architect"},
		"architect",
		&web,
		&workspace,
		&allowMissing,
		&reasoning,
		&autonomy,
		&approval,
		&verify,
		&verifyIterations,
		&verbose,
		&maxChars,
		&localShell,
	)
	if err != nil {
		t.Fatalf("applyExecutionProfile error: %v", err)
	}
	if !architect {
		t.Fatal("expected architect profile to be active")
	}
	if web != "auto" || workspace != "on" || !allowMissing || reasoning != "deep" || autonomy != "on" || approval != "on" || verify != "on" || verifyIterations != 3 || !verbose || maxChars != 2200 || !localShell {
		t.Fatalf("unexpected architect defaults: web=%s workspace=%s allow=%t reasoning=%s autonomy=%s approval=%s verify=%s iter=%d verbose=%t max=%d localShell=%t",
			web, workspace, allowMissing, reasoning, autonomy, approval, verify, verifyIterations, verbose, maxChars, localShell)
	}
}

func TestApplyExecutionProfileArchitectRespectsExplicitFlags(t *testing.T) {
	web := "off"
	workspace := "off"
	allowMissing := false
	reasoning := "fast"
	autonomy := "off"
	approval := "off"
	verify := "off"
	verifyIterations := 1
	verbose := false
	maxChars := 1200
	localShell := false

	args := []string{
		"--profile", "architect",
		"--web", "off",
		"--workspace=off",
		"--allow-missing-tools=false",
		"--reasoning", "fast",
		"--autonomy", "off",
		"--approval=off",
		"--verify", "off",
		"--verify-iterations", "1",
		"--verbose=false",
		"--max-chars=1200",
		"--local-shell=false",
	}

	_, err := applyExecutionProfile(
		args,
		"architect",
		&web,
		&workspace,
		&allowMissing,
		&reasoning,
		&autonomy,
		&approval,
		&verify,
		&verifyIterations,
		&verbose,
		&maxChars,
		&localShell,
	)
	if err != nil {
		t.Fatalf("applyExecutionProfile error: %v", err)
	}

	if web != "off" || workspace != "off" || allowMissing || reasoning != "fast" || autonomy != "off" || approval != "off" || verify != "off" || verifyIterations != 1 || verbose || maxChars != 1200 || localShell {
		t.Fatalf("explicit flag values were overridden unexpectedly")
	}
}

func TestApplyExecutionProfileInvalid(t *testing.T) {
	web := "off"
	_, err := applyExecutionProfile(nil, "invalid", &web, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected invalid profile error")
	}
}
