// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// reuseSlice = telemetry.NewCounter("context_resolver", "reuse_slice",
	// 	nil, "Series split")
	// notReuseSlice = telemetry.NewCounter("context_resolver", "notreuse_slice",
	// 	nil, "Series split")
	k8sTagsCount = telemetry.NewGauge("context_resolver", "k8s_tag_count",
		nil, "TODO")
)

// Context holds the elements that form a context, and can be serialized into a context key
type Context struct {
	Name string
	Tags *metrics.CompositeTags
	Host string
}

// contextResolver allows tracking and expiring contexts
type contextResolver struct {
	contextsByKey map[ckey.ContextKey]*Context
	keyGenerator  *ckey.KeyGenerator
	// buffer slice allocated once per contextResolver to combine and sort
	// tags, origin detection tags and k8s tags.
	tagsBuffer1 *tagset.HashingTagsAccumulator

	tagsBuffer2 *tagset.HashingTagsAccumulator
	// lastContext ckey.ContextKey
	// lastSlice   []string
}

// generateContextKey generates the contextKey associated with the context of the metricSample
func (cr *contextResolver) generateContextKey(metricSampleContext metrics.MetricSampleContext) (ckey.ContextKey, ckey.ContextKey) {
	hash2 := cr.keyGenerator.GenerateHash(cr.tagsBuffer2)
	return cr.keyGenerator.Generate(metricSampleContext.GetName(), metricSampleContext.GetHost(), cr.tagsBuffer1, hash2), ckey.ContextKey(hash2)
}

func newContextResolver() *contextResolver {
	return &contextResolver{
		contextsByKey: make(map[ckey.ContextKey]*Context),
		keyGenerator:  ckey.NewKeyGenerator(),
		tagsBuffer1:   tagset.NewHashingTagsAccumulator(),
		tagsBuffer2:   tagset.NewHashingTagsAccumulator(),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *contextResolver) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	metricSampleContext.GetTags(cr.tagsBuffer1, cr.tagsBuffer2) // tags here are not sorted and can contain duplicates
	//	contextKey, contextKey2Only := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates from cr.tagsBuffer (and doesn't mind the order)
	contextKey, _ := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates from cr.tagsBuffer (and doesn't mind the order)

	if _, ok := cr.contextsByKey[contextKey]; !ok {

		// if contextKey2Only != cr.lastContext {
		// 	cr.lastSlice = cr.tagsBuffer2.Copy()
		// 	cr.lastContext = contextKey2Only
		// 	notReuseSlice.Inc()
		// } else {
		// 	reuseSlice.Inc()
		// }
		tags2 := cr.tagsBuffer2.Copy()
		k8sTagsCount.Set(float64(len(tags2)))

		// making a copy of tags for the context since tagsBuffer
		// will be reused later. This allow us to allocate one slice
		// per context instead of one per sample.
		cr.contextsByKey[contextKey] = &Context{
			Name: metricSampleContext.GetName(),
			Tags: metrics.NewCompositeTags(append(cr.tagsBuffer1.Copy(), tags2...), nil),
			Host: metricSampleContext.GetHost(),
		}
	}

	cr.tagsBuffer1.Reset()
	cr.tagsBuffer2.Reset()
	return contextKey
}

func (cr *contextResolver) get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.contextsByKey[key]
	return ctx, found
}

func (cr *contextResolver) length() int {
	return len(cr.contextsByKey)
}

func (cr *contextResolver) removeKeys(expiredContextKeys []ckey.ContextKey) {
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.contextsByKey, expiredContextKey)
	}
}

// timestampContextResolver allows tracking and expiring contexts based on time.
type timestampContextResolver struct {
	resolver      *contextResolver
	lastSeenByKey map[ckey.ContextKey]float64
}

func newTimestampContextResolver() *timestampContextResolver {
	return &timestampContextResolver{
		resolver:      newContextResolver(),
		lastSeenByKey: make(map[ckey.ContextKey]float64),
	}
}

// updateTrackedContext updates the last seen timestamp on a given context key
func (cr *timestampContextResolver) updateTrackedContext(contextKey ckey.ContextKey, timestamp float64) error {
	if _, ok := cr.lastSeenByKey[contextKey]; ok && cr.lastSeenByKey[contextKey] < timestamp {
		cr.lastSeenByKey[contextKey] = timestamp
	} else if !ok {
		return fmt.Errorf("Trying to update a context that is not tracked")
	}

	return nil
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *timestampContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext, currentTimestamp float64) ckey.ContextKey {
	contextKey := cr.resolver.trackContext(metricSampleContext)
	cr.lastSeenByKey[contextKey] = currentTimestamp
	return contextKey
}

func (cr *timestampContextResolver) length() int {
	return cr.resolver.length()
}

func (cr *timestampContextResolver) get(key ckey.ContextKey) (*Context, bool) {
	return cr.resolver.get(key)
}

// expireContexts cleans up the contexts that haven't been tracked since the given timestamp
// and returns the associated contextKeys
func (cr *timestampContextResolver) expireContexts(expireTimestamp float64) []ckey.ContextKey {
	var expiredContextKeys []ckey.ContextKey

	// Find expired context keys
	for contextKey, lastSeen := range cr.lastSeenByKey {
		if lastSeen < expireTimestamp {
			expiredContextKeys = append(expiredContextKeys, contextKey)
		}
	}

	cr.resolver.removeKeys(expiredContextKeys)

	// Delete expired context keys
	for _, expiredContextKey := range expiredContextKeys {
		delete(cr.lastSeenByKey, expiredContextKey)
	}

	return expiredContextKeys
}

// countBasedContextResolver allows tracking and expiring contexts based on the number
// of calls of `expireContexts`.
type countBasedContextResolver struct {
	resolver            *contextResolver
	expireCountByKey    map[ckey.ContextKey]int64
	expireCount         int64
	expireCountInterval int64
}

func newCountBasedContextResolver(expireCountInterval int) *countBasedContextResolver {
	return &countBasedContextResolver{
		resolver:            newContextResolver(),
		expireCountByKey:    make(map[ckey.ContextKey]int64),
		expireCount:         0,
		expireCountInterval: int64(expireCountInterval),
	}
}

// trackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *countBasedContextResolver) trackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	contextKey := cr.resolver.trackContext(metricSampleContext)
	cr.expireCountByKey[contextKey] = cr.expireCount
	return contextKey
}

func (cr *countBasedContextResolver) get(key ckey.ContextKey) (*Context, bool) {
	return cr.resolver.get(key)
}

// expireContexts cleans up the contexts that haven't been tracked since `expirationCount`
// call to `expireContexts` and returns the associated contextKeys
func (cr *countBasedContextResolver) expireContexts() []ckey.ContextKey {
	var keys []ckey.ContextKey
	for key, index := range cr.expireCountByKey {
		if index <= cr.expireCount-cr.expireCountInterval {
			keys = append(keys, key)
			delete(cr.expireCountByKey, key)
		}
	}
	cr.resolver.removeKeys(keys)
	cr.expireCount++
	return keys
}
