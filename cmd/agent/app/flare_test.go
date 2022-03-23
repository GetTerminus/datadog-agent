// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
)

type mockProfileCollector struct {
	mock.Mock
}

func (m *mockProfileCollector) CreatePerformanceProfile(prefix, debugURL string, cpusec int, target *flare.ProfileData) error {
	args := m.Called(prefix, debugURL, cpusec, target)
	return args.Error(0)
}

func TestReadProfileData(t *testing.T) {
	m := &mockProfileCollector{}
	defer m.AssertExpectations(t)

	mockConfig := config.Mock()
	mockConfig.Set("expvar_port", "1001")
	mockConfig.Set("apm_config.enabled", true)
	mockConfig.Set("apm_config.receiver_port", "1002")
	mockConfig.Set("apm_config.receiver_timeout", "10")
	mockConfig.Set("process_config.expvar_port", "1003")

	pdata := &flare.ProfileData{}

	m.On("CreatePerformanceProfile", "core", "http://127.0.0.1:1001/debug/pprof", 30, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "trace", "http://127.0.0.1:1002/debug/pprof", 9, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "process", "http://127.0.0.1:1003/debug/pprof", 30, pdata).Return(nil)

	err := readProfileData(pdata, 30, m.CreatePerformanceProfile)
	assert.NoError(t, err)
}

func TestReadProfileDataNoTraceAgent(t *testing.T) {
	m := &mockProfileCollector{}
	defer m.AssertExpectations(t)

	mockConfig := config.Mock()
	mockConfig.Set("expvar_port", "1001")
	mockConfig.Set("apm_config.enabled", false)
	mockConfig.Set("process_config.expvar_port", "1003")

	pdata := &flare.ProfileData{}

	m.On("CreatePerformanceProfile", "core", "http://127.0.0.1:1001/debug/pprof", 30, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "process", "http://127.0.0.1:1003/debug/pprof", 30, pdata).Return(nil)

	err := readProfileData(pdata, 30, m.CreatePerformanceProfile)
	assert.NoError(t, err)
}

func TestReadProfileDataErrors(t *testing.T) {
	m := &mockProfileCollector{}
	defer m.AssertExpectations(t)

	mockConfig := config.Mock()
	mockConfig.Set("expvar_port", "1001")
	mockConfig.Set("apm_config.enabled", true)
	mockConfig.Set("apm_config.receiver_port", "1002")
	mockConfig.Set("apm_config.receiver_timeout", "10")
	mockConfig.Set("process_config.expvar_port", "1003")

	pdata := &flare.ProfileData{}

	m.On("CreatePerformanceProfile", "core", "http://127.0.0.1:1001/debug/pprof", 30, pdata).Return(errors.New("can't connect to core agent"))
	m.On("CreatePerformanceProfile", "trace", "http://127.0.0.1:1002/debug/pprof", 9, pdata).Return(errors.New("can't connect to trace agent"))
	m.On("CreatePerformanceProfile", "process", "http://127.0.0.1:1003/debug/pprof", 30, pdata).Return(nil)

	err := readProfileData(pdata, 30, m.CreatePerformanceProfile)
	const expectError = `2 errors occurred:
	* error collecting core agent profile: can't connect to core agent
	* error collecting trace agent profile: can't connect to trace agent

`
	assert.EqualError(t, err, expectError)
}
