// Copyright (c) 2017 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"

	"github.com/ligato/cn-infra/db/keyval"
	"github.com/ligato/cn-infra/statuscheck/model/status"
	"github.com/ligato/vpp-agent/plugins/defaultplugins/ifplugin/model/interfaces"
	"github.com/ligato/vpp-agent/plugins/defaultplugins/l2plugin/model/l2"
	"github.com/ligato/vpp-agent/plugins/defaultplugins/l3plugin/model/l3"
)

// VppMetaData defines the Etcd metadata
type VppMetaData struct {
	Rev int64
	Key string
}

// InterfaceWithMD contains a data record for interface and its
// Etcd metadata
type InterfaceWithMD struct {
	Config *IfConfigWithMD
	State  *IfStateWithMD
}

// IfConfigWithMD contains a data record for interface configuration
// and its Etcd metadata
type IfConfigWithMD struct {
	Metadata  VppMetaData
	Interface *interfaces.Interfaces_Interface
}

// IfStateWithMD contains a data record for interface State and its
// Etcd metadata
type IfStateWithMD struct {
	Metadata       VppMetaData
	InterfaceState *interfaces.InterfacesState_Interface
}

// InterfaceErrorWithMD contains a data record for interface errors and its
// Etcd metadata
type InterfaceErrorWithMD struct {
	VppMetaData
	InterfaceErrorList []*interfaces.InterfaceErrors_Interface
}

// BridgeDomainErrorWithMD contains a data record for bridge domain errors and its
// Etcd metadata
type BridgeDomainErrorWithMD struct {
	VppMetaData
	BdErrorList []*l2.BridgeDomainErrors_BridgeDomain
}

// BdWithMD contains a Bridge Domain data record and its Etcd
// metadata
type BdWithMD struct {
	Config *BdConfigWithMD
	State  *BdStateWithMD
}

// BdConfigWithMD contains a Bridge Domain config data record and its Etcd
// metadata
type BdConfigWithMD struct {
	Metadata     VppMetaData
	BridgeDomain *l2.BridgeDomains_BridgeDomain
}

// BdStateWithMD contains a Bridge Domain state data record and its Etcd
// metadata
type BdStateWithMD struct {
	Metadata          VppMetaData
	BridgeDomainState *l2.BridgeDomainState_BridgeDomain
}

// FibTableWithMD contains a FIB table data record and its Etcd
// metadata
type FibTableWithMD struct {
	VppMetaData
	FibTable []*l2.FibTableEntries_FibTableEntry
}

// XconnectWithMD contains an l2 cross-Connect data record and its
// Etcd metadata
type XconnectWithMD struct {
	VppMetaData
	*l2.XConnectPairs_XConnectPair
}

// StaticRoutesWithMD contains a static route data record and its
// Etcd metadata
type StaticRoutesWithMD struct {
	VppMetaData
	l3.StaticRoutes
}

// VppStatusWithMD contains a VPP Status data record and its Etcd
// metadata
type VppStatusWithMD struct {
	VppMetaData
	status.AgentStatus
}

// VppData defines a structure to hold all Etcd data records (of all
// types) for one VPP.
type VppData struct {
	Interfaces         map[string]InterfaceWithMD
	InterfaceErrors    map[string]InterfaceErrorWithMD
	BridgeDomains      map[string]BdWithMD
	BridgeDomainErrors map[string]BridgeDomainErrorWithMD
	FibTableEntries    FibTableWithMD
	XConnectPairs      map[string]XconnectWithMD
	StaticRoutes       StaticRoutesWithMD
	Status             map[string]VppStatusWithMD
	ShowEtcd           bool
}

// EtcdDump is a map of VppData records. It constitutes a temporary
// storage for data retrieved from Etcd. "Temporary" means during
// the execution of an agentctl command. Every command first reads
// data from Etcd, then processes it, and finally either outputs
// the processed data to the user or updates one or more data records
// in Etcd.
type EtcdDump map[string]*VppData

const (
	stsIDAgent = "Agent"
)

// NewEtcdDump returns a new instance of the temporary storage
// that will hold data retrieved from Etcd.
func NewEtcdDump() EtcdDump {
	return make(EtcdDump)
}

// CreateEmptyRecord creates an empty placeholder record in the
// EtcdDump temporary storage.
func (ed EtcdDump) CreateEmptyRecord(key string) {
	label, _, _, _ := ParseKey(key)
	ed[label] = newVppDataRecord()
}

// ReadDataFromDb reads a data record from Etcd, parses it according to
// the expected record type and stores it in the EtcdDump temporary
// storage. A record is identified by a Key.
//
// The function returns an error if the etcd client encountered an
// error. The function returns true if the specified item has been
// found.
func (ed EtcdDump) ReadDataFromDb(db keyval.ProtoBroker, key string,
	labelFilter []string, typeFilter []string) (found bool, err error) {
	label, dataType, params, plugStatCfgRev := ParseKey(key)
	if !isItemAllowed(label, labelFilter) {
		return false, nil
	}

	if plugStatCfgRev == status.StatusPrefix {
		vd, ok := ed[label]
		if !ok {
			vd = newVppDataRecord()
		}
		ed[label], err = readStatusFromDb(db, vd, key, params)
		return true, err
	}

	if !isItemAllowed(dataType, typeFilter) {
		return false, nil
	}

	vd, ok := ed[label]
	if !ok {
		vd = newVppDataRecord()
	}
	switch dataType {
	case interfaces.InterfacePrefix:
		ed[label], err = readIfConfigFromDb(db, vd, key, params)
	case interfaces.IfStatePrefix:
		ed[label], err = readIfStateFromDb(db, vd, key, params)
	case interfaces.IfErrorPrefix:
		ed[label], err = readInterfaceErrorFromDb(db, vd, key, params)
	case l2.BdPrefix:
		ed[label], err = readBdConfigFromDb(db, vd, key, params)
	case l2.BdStatePrefix:
		ed[label], err = readBdStateFromDb(db, vd, key, params)
	case l2.BdErrPrefix:
		ed[label], err = readBdErrorFromDb(db, vd, key, params)
	case l2.FIBPrefix:
		ed[label], err = readFibFromDb(db, vd, key, params)
	case l2.XconnectPrefix:
		ed[label], err = readXconnectFromDb(db, vd, key, params)
	case l3.RoutesPrefix:
		ed[label], err = readRoutesFromDb(db, vd, key)
	}

	return true, err
}

func isItemAllowed(item string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if strings.Contains(item, f) {
			return true
		}
	}
	return false
}

func readIfConfigFromDb(db keyval.ProtoBroker, vd *VppData, key string, parms []string) (*VppData, error) {
	ifc := &interfaces.Interfaces_Interface{}
	if len(parms) == 0 {
		fmt.Printf("WARNING: Invalid interface Key '%s'\n", key)
		return vd, nil
	}
	found, rev, err := readDataFromDb(db, key, ifc)
	if found && err == nil {
		vd.Interfaces[parms[0]] = InterfaceWithMD{
			Config: &IfConfigWithMD{VppMetaData{rev, key}, ifc},
			State:  vd.Interfaces[parms[0]].State,
		}
	}

	return vd, err
}

func readIfStateFromDb(db keyval.ProtoBroker, vd *VppData, key string, parms []string) (*VppData, error) {
	ifs := &interfaces.InterfacesState_Interface{}
	if len(parms) == 0 {
		fmt.Printf("WARNING: Invalid ifstate Key '%s'\n", key)
		return vd, nil
	}
	found, rev, err := readDataFromDb(db, key, ifs)
	if found && err == nil {
		vd.Interfaces[parms[0]] = InterfaceWithMD{
			Config: (vd.Interfaces[parms[0]]).Config,
			State:  &IfStateWithMD{VppMetaData{rev, key}, ifs},
		}
	}
	return vd, err
}

func readInterfaceErrorFromDb(db keyval.ProtoBroker, vd *VppData, key string, params []string) (*VppData, error) {
	ife := &interfaces.InterfaceErrors_Interface{}
	if len(params) == 0 {
		fmt.Printf("WARNING: Invalid interface Key '%s'\n", key)
		return vd, nil
	}
	found, rev, err := readDataFromDb(db, key, ife)
	if found && err == nil {
		ifaceErrList := vd.InterfaceErrors[params[0]].InterfaceErrorList
		ifaceErrList = append(ifaceErrList, ife)
		vd.InterfaceErrors[params[0]] = InterfaceErrorWithMD{
			VppMetaData{rev, key}, ifaceErrList,
		}
	}

	return vd, err
}

func readBdConfigFromDb(db keyval.ProtoBroker, vd *VppData, key string, parms []string) (*VppData, error) {
	if len(parms) == 0 {
		fmt.Printf("WARNING: Invalid bridge domain config Key '%s'\n", key)
		return vd, nil
	}
	bd := &l2.BridgeDomains_BridgeDomain{}
	found, rev, err := readDataFromDb(db, key, bd)
	if found && err == nil {
		vd.BridgeDomains[parms[0]] = BdWithMD{
			Config: &BdConfigWithMD{VppMetaData{rev, key}, bd},
			State:  vd.BridgeDomains[parms[0]].State,
		}
	}
	return vd, err
}

func readBdStateFromDb(db keyval.ProtoBroker, vd *VppData, key string, parms []string) (*VppData, error) {
	if len(parms) == 0 {
		fmt.Printf("WARNING: Invalid bridge domain state Key '%s'\n", key)
		return vd, nil
	}
	bd := &l2.BridgeDomainState_BridgeDomain{}
	found, rev, err := readDataFromDb(db, key, bd)
	if found && err == nil {
		vd.BridgeDomains[parms[0]] = BdWithMD{
			Config: vd.BridgeDomains[parms[0]].Config,
			State:  &BdStateWithMD{VppMetaData{rev, key}, bd},
		}
	}
	return vd, err
}

func readBdErrorFromDb(db keyval.ProtoBroker, vd *VppData, key string, params []string) (*VppData, error) {
	bde := &l2.BridgeDomainErrors_BridgeDomain{}
	if len(params) == 0 {
		fmt.Printf("WARNING: Invalid interface Key '%s'\n", key)
		return vd, nil
	}
	found, rev, err := readDataFromDb(db, key, bde)
	if found && err == nil {
		bdErrList := vd.BridgeDomainErrors[params[0]].BdErrorList
		bdErrList = append(bdErrList, bde)
		vd.BridgeDomainErrors[params[0]] = BridgeDomainErrorWithMD{
			VppMetaData{rev, key}, bdErrList,
		}
	}

	return vd, err
}

func readFibFromDb(db keyval.ProtoBroker, vd *VppData, key string, parms []string) (*VppData, error) {
	if len(parms) == 0 {
		fmt.Printf("WARNING: Invalid FIB Key '%s'\n", key)
		return vd, nil
	}
	fibEntry := &l2.FibTableEntries_FibTableEntry{}
	found, rev, err := readDataFromDb(db, key, fibEntry)
	if found && err == nil {
		fibTable := vd.FibTableEntries.FibTable
		fibTable = append(fibTable, fibEntry)
		vd.FibTableEntries =
			FibTableWithMD{VppMetaData{rev, key}, fibTable}
	}
	return vd, err
}

func readXconnectFromDb(db keyval.ProtoBroker, vd *VppData, key string, parms []string) (*VppData, error) {
	if len(parms) == 0 {
		fmt.Printf("WARNING: Invalid cross-connect Key '%s'\n", key)
		return vd, nil
	}
	xc := &l2.XConnectPairs_XConnectPair{}
	found, rev, err := readDataFromDb(db, key, xc)
	if found && err == nil {
		vd.XConnectPairs[parms[0]] =
			XconnectWithMD{VppMetaData{rev, key}, xc}
	}
	return vd, err
}

func readRoutesFromDb(db keyval.ProtoBroker, vd *VppData, key string) (*VppData, error) {
	routes := l3.StaticRoutes{}
	found, rev, err := readDataFromDb(db, key, &routes)
	if found && err == nil {
		vd.StaticRoutes =
			StaticRoutesWithMD{VppMetaData{rev, key}, routes}
	}
	return vd, err
}

func readStatusFromDb(db keyval.ProtoBroker, vd *VppData, key string, parms []string) (*VppData, error) {
	id := stsIDAgent
	if len(parms) > 0 {
		id = parms[0]
	}
	sts := status.AgentStatus{}
	found, rev, err := readDataFromDb(db, key, &sts)
	if found && err == nil {
		vd.Status[id] =
			VppStatusWithMD{VppMetaData{rev, key}, sts}
	}
	return vd, err
}

func readDataFromDb(db keyval.ProtoBroker, key string, obj proto.Message) (bool, int64, error) {
	found, rev, err := db.GetValue(key, obj)
	if err != nil {
		return false, rev, errors.New("Could not read from database, Key:" + key + ", error" + err.Error())
	}
	if !found {
		fmt.Printf("WARNING: data for Key '%s' not found\n", key)
	}
	return found, rev, nil
}

// DeleteDataFromDb deletes the specified Key from the database, if
// the Key matches both the labelFilter and the dataFilter.
//
// The function returns an error if the etcd client encountered an
// error. The function returns true if the specified item has been
// found and successfully deleted.
func DeleteDataFromDb(db keyval.ProtoBroker, key string,
	labelFilter []string, typeFilter []string) (bool, error) {
	label, dataType, _, _ := ParseKey(key)
	if !isItemAllowed(label, labelFilter) {
		return false, nil
	}
	if !isItemAllowed(dataType, typeFilter) {
		return false, nil
	}
	return db.Delete(key)
}

func newVppDataRecord() *VppData {
	return &VppData{
		Interfaces:         make(map[string]InterfaceWithMD),
		InterfaceErrors:    make(map[string]InterfaceErrorWithMD),
		BridgeDomains:      make(map[string]BdWithMD),
		BridgeDomainErrors: make(map[string]BridgeDomainErrorWithMD),
		FibTableEntries:    FibTableWithMD{},
		XConnectPairs:      make(map[string]XconnectWithMD),
		StaticRoutes:       StaticRoutesWithMD{},
		Status:             make(map[string]VppStatusWithMD),
		ShowEtcd:           false,
	}
}

func (ed EtcdDump) getSortedKeys() []string {
	keys := []string{}
	for k := range ed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
