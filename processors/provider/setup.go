package provider

import (
	"fmt"

	"github.com/jcelliott/lumber"

	"github.com/nanobox-io/nanobox/models"
	"github.com/nanobox-io/nanobox/util/provider"
	"github.com/nanobox-io/nanobox/util/dhcp"
	"github.com/nanobox-io/nanobox/util/locker"
	"github.com/nanobox-io/nanobox/util/display"
)

// Setup sets up the provider (launch VM, etc)
func Setup() error {
	locker.GlobalLock()
	defer locker.GlobalUnlock()

	display.OpenContext("Preparing Nanobox")

	// create the provider (VM)
	if err := provider.Create(); err != nil {
		lumber.Error("provider:Setup:provider.Create(): %s", err.Error())
		return fmt.Errorf("failed to create the provider: %s", err.Error())
	}

	// start the provider (VM)
	if err := provider.Start(); err != nil {
		lumber.Error("provider:Setup:provider.Start(): %s", err.Error())
		return fmt.Errorf("failed to start the provider: %s", err.Error())
	}

	// attach the network to the host stack
	if err := setupNetwork(); err != nil {
		return fmt.Errorf("failed to setup the provider network: %s", err.Error())
	}

	// initialize the docker client
	if err := Init(); err != nil {
		return fmt.Errorf("failed to initialize docker for provider: %s", err.Error())
	}

	display.CloseContext()
	
	return nil
}

// setupNetwork sets up the provider network
func setupNetwork() error {
	// fetch the provider model
	model, _ := models.LoadProvider()
	
	// short-circuit if this is already done
	if model.HostIP != "" {
		return nil
	}

	display.StartTask("Joining virtual network")
	
	// reserve an IP to be used for mounting
	mountIP, err := dhcp.ReserveGlobal()
	if err != nil {
		display.ErrorTask()
		lumber.Error("provider:Setup:setupNetwork:dhcp.ReserveGlobal(): %s", err.Error())
		return fmt.Errorf("failed to reserve a global IP: %s", err.Error())
	}
	
	// add the mount IP to the provider
	if err := provider.AddIP(mountIP.String()); err != nil {
		display.ErrorTask()
		lumber.Error("provider:Setup:setupNetwork:provider.AddIP(%s): %s", mountIP, err.Error())
		return fmt.Errorf("failed to add an IP to the provider for mounting: %s", err.Error())
	}
	
	// set the mount IP as the default gateway
	if err := provider.SetDefaultIP(mountIP.String()); err != nil {
		display.ErrorTask()
		lumber.Error("provider:Setup:setupNetwork:provider.SetDefaultIP(%s): %s", mountIP, err.Error())
		return fmt.Errorf("failed to set the mount IP as the default gateway: %s", err.Error())
	}
	
	// retrieve the provider's Host IP
	hostIP, err := provider.HostIP()
	if err != nil {
		display.ErrorTask()
		lumber.Error("provider:Setup:setupNetwork:provider.HostIP(): %s", err.Error())
		return fmt.Errorf("unable to retrieve the host IP from the provider: %s", err.Error())
	}
	
	// persist the IPs for later use
	model.MountIP = mountIP.String()
	model.HostIP  = hostIP
	if err := model.Save(); err != nil {
		display.ErrorTask()
		return fmt.Errorf("failed to persist the provider model: %s", err.Error())
	}
	
	display.StopTask()
	
	return nil
}
