package code

import (
	"fmt"

	"github.com/jcelliott/lumber"
	"github.com/nanobox-io/golang-docker-client"

	"github.com/nanobox-io/nanobox-boxfile"
	"github.com/nanobox-io/nanobox/commands/registry"
	container_generator "github.com/nanobox-io/nanobox/generators/containers"
	hook_generator "github.com/nanobox-io/nanobox/generators/hooks/build"

	"github.com/nanobox-io/nanobox/models"
	"github.com/nanobox-io/nanobox/util"
	"github.com/nanobox-io/nanobox/util/config"
	"github.com/nanobox-io/nanobox/util/display"
	"github.com/nanobox-io/nanobox/util/hookit"
)

// Build builds the codebase that can then be deployed
func Build(envModel *models.Env) error {
	display.OpenContext("Building application")
	defer display.CloseContext()

	// pull the latest build image
	buildImage, err := pullBuildImage()
	if err != nil {
		return fmt.Errorf("failed to pull the build image: %s", err.Error())
	}

	// if a build container was leftover from a previous build, let's remove it
	docker.ContainerRemove(container_generator.BuildName())

	display.StartTask("Starting docker container")

	// start the container
	config := container_generator.BuildConfig(buildImage)
	container, err := docker.CreateContainer(config)
	if err != nil {
		lumber.Error("code:Build:docker.CreateContainer(%+v): %s", config, err.Error())
		return fmt.Errorf("failed to start docker container: %s", err.Error())
	}

	display.StopTask()

	// ensure we stop the container when we're done
	defer docker.ContainerRemove(container_generator.BuildName())

	if err := prepareEnvironment(container.ID); err != nil {
		return err
	}

	if err := gatherRequirements(envModel, container.ID); err != nil {
		return err
	}

	if err := installRuntimes(container.ID); err != nil {
		return err
	}

	if err := compileCode(container.ID); err != nil {
		return err
	}

	if err := packageBuild(container.ID); err != nil {
		return err
	}

	return nil
}

// prepareEnvironment runs hooks to prepare the build environment
func prepareEnvironment(containerID string) error {
	display.StartTask("Preparing environment for build")
	defer display.StopTask()

	// run the user hook
	if _, err := hookit.RunUserHook(containerID, hook_generator.UserPayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	// run the configure hook
	if _, err := hookit.RunConfigureHook(containerID, hook_generator.ConfigurePayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	// run the fetch hook
	if _, err := hookit.RunFetchHook(containerID, hook_generator.FetchPayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	// run the setup hook
	if _, err := hookit.RunSetupHook(containerID, hook_generator.SetupPayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	return nil
}

// gatherRequirements runs hooks to gather requirements
func gatherRequirements(envModel *models.Env, containerID string) error {
	display.StartTask("Gathering requirements")
	defer display.StopTask()

	// run the boxfile hook
	boxOutput, err := hookit.RunBoxfileHook(containerID, hook_generator.BoxfilePayload())
	if err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	box := boxfile.NewFromPath(config.Boxfile())

	// persist the boxfile output to the env model
	envModel.UserBoxfile = box.String()
	envModel.BuiltBoxfile = boxOutput
	envModel.BuiltID = util.RandomString(30)
	if err := envModel.Save(); err != nil {
		display.ErrorTask()
		lumber.Error("code:Build:models:Env:Save(): %s", err.Error())
		return fmt.Errorf("failed to persist build boxfile to db: %s", err.Error())
	}

	return nil
}

// installRuntimes runs the hooks to install binaries and runtimes
func installRuntimes(containerID string) error {
	display.StartTask("Installing binaries and runtimes")
	defer display.StopTask()

	// run the prepare hook
	if _, err := hookit.RunPrepareHook(containerID, hook_generator.PreparePayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	return nil
}

// compileCode runs the hooks to compile the codebase
func compileCode(containerID string) error {
	if registry.GetBool("skip-compile") {
		return nil
	}

	display.StartTask("Compiling code")
	defer display.StopTask()

	// run the compile hook
	if _, err := hookit.RunCompileHook(containerID, hook_generator.CompilePayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	// run the pack-app hook
	if _, err := hookit.RunPackAppHook(containerID, hook_generator.PackAppPayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	return nil
}

// packageBuild runs the hooks to package the build
func packageBuild(containerID string) error {
	display.StartTask("Packaging build")
	defer display.StopTask()

	// run the pack-build hook
	if _, err := hookit.RunPackBuildHook(containerID, hook_generator.PackBuildPayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	if registry.GetBool("skip-compile") {
		return nil
	}

	// run the clean hook
	if _, err := hookit.RunCleanHook(containerID, hook_generator.CleanPayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	// run the pack-deploy hook
	if _, err := hookit.RunPackDeployHook(containerID, hook_generator.PackDeployPayload()); err != nil {
		display.ErrorTask()
		return runDebugSession(containerID, err)
	}

	return nil
}
