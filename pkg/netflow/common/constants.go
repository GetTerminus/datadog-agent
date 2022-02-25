// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

const (
	// DefaultStopTimeout is the default stop timeout
	DefaultStopTimeout = 5

	// DefaultAggregatorFlushInterval is the default flush interval
	DefaultAggregatorFlushInterval = 10

	// DefaultAggregatorBufferSize is the default aggregator buffer size interval
	DefaultAggregatorBufferSize = 100
)

// Default ports
const (
	DefaultPortNETFLOW = uint16(2055)
	DefaultPortIPFIX   = uint16(4739)
	DefaultPortSFLOW   = uint16(6343)
)

// FlowType represent the flow protocol (netflow5,netflow9,ipfix, sflow, etc)
type FlowType string

// Flow types
const (
	TypeIPFIX    FlowType = "ipfix"
	TypeSFlow5            = "sflow5"
	TypeNetFlow5          = "netflow5"
	TypeNetFlow9          = "netflow9"
	TypeUnknown           = "unknown"
)
