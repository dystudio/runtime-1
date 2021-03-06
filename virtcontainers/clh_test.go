// Copyright (c) 2019 Ericsson Eurolab Deutschland G.m.b.H.
//
// SPDX-License-Identifier: Apache-2.0
//

package virtcontainers

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/kata-containers/runtime/virtcontainers/device/config"
	chclient "github.com/kata-containers/runtime/virtcontainers/pkg/cloud-hypervisor/client"
	"github.com/kata-containers/runtime/virtcontainers/store"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func newClhConfig() (HypervisorConfig, error) {

	setupClh()

	if testClhPath == "" {
		return HypervisorConfig{}, errors.New("hypervisor fake path is empty")
	}

	if testVirtiofsdPath == "" {
		return HypervisorConfig{}, errors.New("hypervisor fake path is empty")
	}

	if _, err := os.Stat(testClhPath); os.IsNotExist(err) {
		return HypervisorConfig{}, err
	}

	if _, err := os.Stat(testVirtiofsdPath); os.IsNotExist(err) {
		return HypervisorConfig{}, err
	}

	return HypervisorConfig{
		KernelPath:        testClhKernelPath,
		ImagePath:         testClhImagePath,
		HypervisorPath:    testClhPath,
		NumVCPUs:          defaultVCPUs,
		BlockDeviceDriver: config.VirtioBlock,
		MemorySize:        defaultMemSzMiB,
		DefaultBridges:    defaultBridges,
		DefaultMaxVCPUs:   MaxClhVCPUs(),
		SharedFS:          config.VirtioFS,
		VirtioFSCache:     virtioFsCacheAlways,
		VirtioFSDaemon:    testVirtiofsdPath,
	}, nil
}

type clhClientMock struct {
	vmInfo chclient.VmInfo
}

func (c *clhClientMock) VmmPingGet(ctx context.Context) (chclient.VmmPingResponse, *http.Response, error) {
	return chclient.VmmPingResponse{}, nil, nil
}

func (c *clhClientMock) ShutdownVMM(ctx context.Context) (*http.Response, error) {
	return nil, nil
}

func (c *clhClientMock) CreateVM(ctx context.Context, vmConfig chclient.VmConfig) (*http.Response, error) {
	c.vmInfo.State = clhStateCreated
	return nil, nil
}

//nolint:golint
func (c *clhClientMock) VmInfoGet(ctx context.Context) (chclient.VmInfo, *http.Response, error) {
	return c.vmInfo, nil, nil
}

func (c *clhClientMock) BootVM(ctx context.Context) (*http.Response, error) {
	c.vmInfo.State = clhStateRunning
	return nil, nil
}

func TestCloudHypervisorAddVSock(t *testing.T) {
	assert := assert.New(t)
	clh := cloudHypervisor{}

	clh.addVSock(1, "path")
	assert.Equal(clh.vmconfig.Vsock[0].Cid, int64(1))
	assert.Equal(clh.vmconfig.Vsock[0].Sock, "path")

	clh.addVSock(2, "path2")
	assert.Equal(clh.vmconfig.Vsock[1].Cid, int64(2))
	assert.Equal(clh.vmconfig.Vsock[1].Sock, "path2")
}

// Check addNet appends to the network config list new configurations.
// Check that the elements in the list has the correct values
func TestCloudHypervisorAddNetCheckNetConfigListValues(t *testing.T) {
	macTest := "00:00:00:00:00"
	tapPath := "/path/to/tap"

	assert := assert.New(t)

	clh := cloudHypervisor{}

	e := &VethEndpoint{}
	e.NetPair.TAPIface.HardAddr = macTest
	e.NetPair.TapInterface.TAPIface.Name = tapPath

	err := clh.addNet(e)
	assert.Nil(err)

	assert.Equal(len(clh.vmconfig.Net), 1)
	if err == nil {
		assert.Equal(clh.vmconfig.Net[0].Mac, macTest)
		assert.Equal(clh.vmconfig.Net[0].Tap, tapPath)
	}

	err = clh.addNet(e)
	assert.Nil(err)

	assert.Equal(len(clh.vmconfig.Net), 2)
	if err == nil {
		assert.Equal(clh.vmconfig.Net[1].Mac, macTest)
		assert.Equal(clh.vmconfig.Net[1].Tap, tapPath)
	}
}

// Check addNet with valid values, and fail with invalid values
// For Cloud Hypervisor only tap is be required
func TestCloudHypervisorAddNetCheckEnpointTypes(t *testing.T) {
	assert := assert.New(t)

	tapPath := "/path/to/tap"

	validVeth := &VethEndpoint{}
	validVeth.NetPair.TapInterface.TAPIface.Name = tapPath

	type args struct {
		e Endpoint
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"TapEndpoint", args{e: &TapEndpoint{}}, true},
		{"Empty VethEndpoint", args{e: &VethEndpoint{}}, true},
		{"Valid VethEndpoint", args{e: validVeth}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clh := &cloudHypervisor{}
			if err := clh.addNet(tt.args.e); (err != nil) != tt.wantErr {
				t.Errorf("cloudHypervisor.addNet() error = %v, wantErr %v", err, tt.wantErr)

			} else if err == nil {
				assert.Equal(clh.vmconfig.Net[0].Tap, tapPath)
			}
		})
	}
}

func TestCloudHypervisorBootVM(t *testing.T) {
	clh := &cloudHypervisor{}
	clh.APIClient = &clhClientMock{}
	var ctx context.Context
	if err := clh.bootVM(ctx); err != nil {
		t.Errorf("cloudHypervisor.bootVM() error = %v", err)
	}
}

func TestCloudHypervisorCleanupVM(t *testing.T) {
	clh := &cloudHypervisor{}

	if err := clh.cleanupVM(true); err == nil {
		t.Errorf("cloudHypervisor.cleanupVM() expected error != %v", err)
	}

	clh.id = "cleanVMID"

	if err := clh.cleanupVM(true); err != nil {
		t.Errorf("cloudHypervisor.cleanupVM() expected error != %v", err)
	}

	dir := filepath.Join(store.RunVMStoragePath(), clh.id)
	os.MkdirAll(dir, os.ModePerm)

	if err := clh.cleanupVM(false); err != nil {
		t.Errorf("cloudHypervisor.cleanupVM() expected error != %v", err)
	}
	_, err := os.Stat(dir)

	if err == nil {
		t.Errorf("dir should not exist %s", dir)
	}

	if !os.IsNotExist(err) {
		t.Errorf("Unexpected error = %v", err)
	}
}

func TestClhCreateSandbox(t *testing.T) {
	assert := assert.New(t)

	clhConfig, err := newClhConfig()
	assert.NoError(err)

	clh := &cloudHypervisor{
		config: clhConfig,
	}

	sandbox := &Sandbox{
		ctx: context.Background(),
		id:  "testSandbox",
		config: &SandboxConfig{
			HypervisorConfig: clhConfig,
		},
	}

	vcStore, err := store.NewVCSandboxStore(sandbox.ctx, sandbox.id)
	assert.NoError(err)

	sandbox.store = vcStore

	// Create parent dir path for hypervisor.json
	parentDir := store.SandboxConfigurationRootPath(sandbox.id)
	assert.NoError(os.MkdirAll(parentDir, store.DirMode))

	err = clh.createSandbox(context.Background(), sandbox.id, NetworkNamespace{}, &sandbox.config.HypervisorConfig, sandbox.store, false)
	assert.NoError(err)
	assert.NoError(os.RemoveAll(parentDir))
	assert.Exactly(clhConfig, clh.config)
}

func TestClooudHypervisorStartSandbox(t *testing.T) {
	assert := assert.New(t)
	clhConfig, err := newClhConfig()
	assert.NoError(err)

	clh := &cloudHypervisor{
		config:    clhConfig,
		APIClient: &clhClientMock{},
		virtiofsd: &virtiofsdMock{},
	}

	sandbox := &Sandbox{
		ctx: context.Background(),
		id:  "testSandbox",
		config: &SandboxConfig{
			HypervisorConfig: clhConfig,
		},
	}

	vcStore, err := store.NewVCSandboxStore(sandbox.ctx, sandbox.id)
	assert.NoError(err)

	sandbox.store = vcStore

	// Create parent dir path for hypervisor.json
	parentDir := store.SandboxConfigurationRootPath(sandbox.id)
	assert.NoError(os.MkdirAll(parentDir, store.DirMode))

	err = clh.startSandbox(10)
	assert.NoError(err)
}
