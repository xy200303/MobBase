package app

import (
	"context"
	"fmt"

	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/system"
)

func (r runtime) test(ctx context.Context, args []string) error {
	options, err := parseBuild(args)
	if err != nil {
		return err
	}
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	if currentProject == nil {
		return &codedError{Code: "MOB_PROJECT_UNRECOGNIZED", Message: "The current directory is not a supported mobile project."}
	}
	platform, err := selectBuildPlatform(currentProject, options.Platform)
	if err != nil {
		return err
	}
	if platform != "android" || (currentProject.Kind != project.KindAndroid && currentProject.Kind != project.KindFlutter) {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Android is the only test platform implemented in this Mob release."}
	}
	sdk, requirements, err := r.prepareAndroidSDK(ctx, currentProject.Root, "mob test", false, options.NoInstall, options.AcceptLicenses)
	if err != nil {
		return err
	}
	java, err := r.selectProjectJava(ctx, requirements.JavaVersion, options.NoInstall)
	if err != nil {
		return err
	}
	if err := r.emit("started", "mob test", true, map[string]string{"phase": "test", "platform": "android", "sdk": sdk.Name}, nil); err != nil {
		return err
	}
	if currentProject.Kind == project.KindFlutter && len(options.Command) == 0 {
		if _, err := r.ensureFlutterRunner(ctx, currentProject.Root, options.NoInstall, "mob test"); err != nil {
			return err
		}
	}
	program, commandArgs, err := testProjectCommand(currentProject, options.Command)
	if err != nil {
		return err
	}
	program, commandArgs = system.BatchCommand(program, commandArgs...)
	environment := append(androidEnvironment(sdk), javaEnvironment(java)...)
	result, commandErr := r.executeWorkflowCommand(ctx, "mob test", program, commandArgs, environment, currentProject.Root)
	if result.Output != "" {
		if r.json {
			if err := r.emit("log", "mob test", true, map[string]string{"stream": "combined", "output": result.Output}, nil); err != nil {
				return err
			}
		} else {
			fmt.Fprint(r.out, result.Output)
		}
	}
	if commandErr != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Mobile tests failed: " + commandErr.Error(), Remediation: "Review the test output and selected Android SDK."}
	}
	data := map[string]interface{}{"platform": "android", "project": currentProject.Root, "sdk": sdk.Name, "java": java, "command": append([]string{program}, commandArgs...)}
	if r.json {
		return r.result("mob test", data)
	}
	fmt.Fprintln(r.out, "Mobile tests completed.")
	return nil
}

func testProjectCommand(info *project.Info, forwarded []string) (string, []string, error) {
	if len(forwarded) > 0 {
		return forwarded[0], forwarded[1:], nil
	}
	if info.Kind == project.KindFlutter {
		runner, err := flutterRunner(info.Root)
		if err != nil {
			return "", nil, err
		}
		return runner.Program, append(runner.Prefix, "test"), nil
	}
	program, _, err := buildCommand(info.Root, nil)
	if err != nil {
		return "", nil, err
	}
	return program, []string{"test"}, nil
}
