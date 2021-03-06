// Copyright 2017 Politecnico di Torino
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package l2switch

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/mvbpolito/gosexy/to"

	"github.com/iovisor/iovisor-ovn/iomodules"

	"github.com/iovisor/iovisor-ovn/hover"
	l "github.com/op/go-logging"
)

var log = l.MustGetLogger("iomodules-switch")

type L2SwitchModule struct {
	ModuleId   string

	Interfaces map[string]*L2SwitchModuleInterface

	deployed  bool
	hc *hover.Client // used to send commands to hover
}

type L2SwitchModuleInterface struct {
	IfaceId      int    // Iface id inside hover
	LinkIdHover  string // iomodules Link Id
	IfaceName    string
}

func Create(hc *hover.Client) *L2SwitchModule {

	if hc == nil {
		log.Errorf("HoverClient is not valid")
		return nil
	}

	x := new(L2SwitchModule)
	x.Interfaces = make(map[string]*L2SwitchModuleInterface)
	x.hc = hc
	x.deployed = false
	return x
}

func (sw *L2SwitchModule) GetModuleId() string {
	return sw.ModuleId
}

func (sw *L2SwitchModule) Deploy() (err error) {

	if sw.deployed {
		return nil
	}

	switchError, switchHover := sw.hc.ModulePOST("bpf",
		"Switch", SwitchSecurityPolicy)
	if switchError != nil {
		log.Errorf("Error in POST Switch IOModule: %s\n", switchError)
		return switchError
	}

	log.Noticef("POST Switch IOModule %s\n", switchHover.Id)
	sw.ModuleId = switchHover.Id
	sw.deployed = true

	return nil
}

func (sw *L2SwitchModule) Destroy() (err error) {

	if !sw.deployed {
		return nil
	}

	moduleDeleteError, _ := sw.hc.ModuleDELETE(sw.ModuleId)
	if moduleDeleteError != nil {
		log.Errorf("Error in destrying Switch IOModule: %s\n", moduleDeleteError)
		return moduleDeleteError
	}

	sw.ModuleId = ""
	sw.deployed = false

	return nil
}

func (sw *L2SwitchModule) AttachExternalInterface(ifaceName string) (err error) {

	if !sw.deployed {
		errString := "Trying to attach port in undeployed switch"
		log.Errorf(errString)
		return errors.New(errString)
	}

	linkError, linkHover := sw.hc.LinkPOST("i:"+ifaceName, sw.ModuleId)
	if linkError != nil {
		log.Errorf("Error in POSTing the Link: %s\n", linkError)
		return linkError
	}

	if err != nil {
		log.Errorf("Error in finding free port: %s\n", err)
		return err
	}

	// get interface id
	ifacenumber := -1
	if linkHover.From[0:2] == "m:" {
		ifacenumber = linkHover.FromId
	}
	if linkHover.To[0:2] == "m:" {
		ifacenumber = linkHover.ToId
	}
	if ifacenumber == -1 {
		log.Warningf("IfaceId == -1 something wrong happened...\n")
	}

	iface := new(L2SwitchModuleInterface)

	iface.IfaceId = ifacenumber
	iface.LinkIdHover = linkHover.Id
	iface.IfaceName = ifaceName
	sw.Interfaces[ifaceName] = iface

	// TODO: security policies

	return nil
}

func (sw *L2SwitchModule) DetachExternalInterface(ifaceName string) (err error) {

	if !sw.deployed {
		errString := "Trying to detach port in undeployed switch"
		log.Errorf(errString)
		return errors.New(errString)
	}

	iface, ok := sw.Interfaces[ifaceName]

	if !ok {
		errString := fmt.Sprintf("Iface '%s' is not present in switch '%s'\n",
			ifaceName, sw.ModuleId)
		log.Warningf(errString)
		return errors.New(errString)
	}

	linkDeleteError, _ := sw.hc.LinkDELETE(iface.LinkIdHover)

	if linkDeleteError != nil {
		log.Warningf("Problem removing iface '%s' from switch '%s'\n",
			ifaceName, sw.ModuleId)
		return linkDeleteError
	}

	// TODO: clean up port security tables
	delete(sw.Interfaces, ifaceName)

	return nil
}

func (sw *L2SwitchModule) AttachToIoModule(ifaceId int, ifaceName string) (err error) {
	if !sw.deployed {
		log.Errorf("Trying to attach port in undeployed switch\n")
		return errors.New("Trying to attach port in undeployed switch")
	}

	iface := new(L2SwitchModuleInterface)

	iface.IfaceId = ifaceId
	iface.IfaceName = ifaceName

	sw.Interfaces[ifaceName] = iface

	// TODO: security policies
	return nil
}

func (sw *L2SwitchModule) DetachFromIoModule(ifaceName string) (err error) {
	if !sw.deployed {
		log.Errorf("Trying to detach port in undeployed switch\n")
		return errors.New("Trying to detach port in undeployed switch")
	}

	_, ok := sw.Interfaces[ifaceName]

	if !ok {
		errString := fmt.Sprintf("Iface '%s' is not present in switch '%s'\n",
			ifaceName, sw.ModuleId)
		log.Warningf(errString)
		return errors.New(errString)
	}

	// TODO: clean up port security tables

	delete(sw.Interfaces, ifaceName)

	return nil
}

// adds a entry in the forwarding table of the switch
func (sw *L2SwitchModule) AddForwardingTableEntry(mac net.HardwareAddr, ifaceName string) (err error) {

	swIface, ok := sw.Interfaces[ifaceName]
	if !ok {
		errString := fmt.Sprintf("Iface '%s' is not present in switch '%s'\n",
			ifaceName, sw.ModuleId)
		log.Warningf(errString)
		return errors.New(errString)
	}

	macString := "{" + iomodules.MacToHexadecimalStringBigEndian(mac) + "}"

	sw.hc.TableEntryPOST(sw.ModuleId, "fwdtable", macString,
		strconv.Itoa(swIface.IfaceId))

	return nil
}

func (sw *L2SwitchModule) AddPortSecurityMac(mac net.HardwareAddr, ifaceName string) (err error) {

	swIface, ok := sw.Interfaces[ifaceName]
	if !ok {
		errString := fmt.Sprintf("Iface '%s' is not present in switch '%s'\n",
			ifaceName, sw.ModuleId)
		log.Warningf(errString)
		return errors.New(errString)
	}

	macString := iomodules.MacToHexadecimalStringBigEndian(mac)

	sw.hc.TableEntryPOST(sw.ModuleId, "securitymac",
		"{0x"+strconv.Itoa(swIface.IfaceId)+"}", macString)
	return nil
}

func (sw *L2SwitchModule) Configure(conf interface{}) (err error) {
	// The interface is a map with the following elements:
	// forwarding_table: a list of maps, each one has:
	//		port: the port where mac can be reached
	//		mac: the mac itself
	// TODO: support for port security policies
	log.Infof("Configuring Switch")
	confMap := to.Map(conf)
	if fwd_table, ok := confMap["forwarding_table"]; ok {
		for _, entry := range to.List(fwd_table) {
			entryMap := to.Map(entry)

			port_, ok1 := entryMap["port"]
			mac_, ok2 := entryMap["mac"]
			if !ok1 || !ok2 {
				log.Errorf("Skipping non valid forwarding table entry")
				continue
			}

			log.Infof("Adding forwardig table entry '%s' -> '%s'",
				mac_.(string), port_.(string))

			mac, err1 := net.ParseMAC(mac_.(string))
			if err1 != nil {
				log.Errorf("'%s' is not a valid mac address", mac_)
				continue
			}

			err := sw.AddForwardingTableEntry(mac, port_.(string))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// TODO: port security policies
