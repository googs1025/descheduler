/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakemetricsclient "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/test"
)

func TestWaitForLowNodeUtilizationMetricsTimesOut(t *testing.T) {
	metricsClient := fakemetricsclient.NewSimpleClientset()
	nodeMetricsResource := schema.GroupVersionResource{
		Group:    "metrics.k8s.io",
		Version:  "v1beta1",
		Resource: "nodes",
	}
	if err := metricsClient.Tracker().Create(nodeMetricsResource, test.BuildNodeMetrics("kind-worker", 8, 0), ""); err != nil {
		t.Fatalf("unable to create node metrics: %v", err)
	}

	err := waitForLowNodeUtilizationMetrics(
		context.Background(),
		t,
		metricsClient,
		"kind-worker",
		"default",
		12_000,
		100*time.Millisecond,
		10*time.Millisecond,
	)
	if err == nil {
		t.Fatalf("expected timeout while waiting for metrics threshold")
	}
	if !strings.Contains(err.Error(), "timed out waiting for metrics") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestWaitForLowNodeUtilizationMetricsReturnsWhenMetricsReachThreshold(t *testing.T) {
	metricsClient := fakemetricsclient.NewSimpleClientset()
	nodeMetricsResource := schema.GroupVersionResource{
		Group:    "metrics.k8s.io",
		Version:  "v1beta1",
		Resource: "nodes",
	}
	podMetricsResource := schema.GroupVersionResource{
		Group:    "metrics.k8s.io",
		Version:  "v1beta1",
		Resource: "pods",
	}
	if err := metricsClient.Tracker().Create(nodeMetricsResource, test.BuildNodeMetrics("kind-worker", 13_000, 0), ""); err != nil {
		t.Fatalf("unable to create node metrics: %v", err)
	}
	if err := metricsClient.Tracker().Create(podMetricsResource, test.BuildPodMetrics("load-pod", 13_000, 0), "default"); err != nil {
		t.Fatalf("unable to create pod metrics: %v", err)
	}

	err := waitForLowNodeUtilizationMetrics(
		context.Background(),
		t,
		metricsClient,
		"kind-worker",
		"default",
		12_000,
		time.Second,
		time.Millisecond,
	)
	if err != nil {
		t.Fatalf("expected metrics threshold to be reached, got %v", err)
	}
}

func TestMinCPUUsageMilliForThreshold(t *testing.T) {
	node := test.BuildTestNode("kind-worker", 32_000, 0, 0, nil)

	tests := []struct {
		name      string
		node      *v1.Node
		threshold api.Percentage
		expected  int64
	}{
		{
			name:      "partial threshold",
			node:      node,
			threshold: 25,
			expected:  8_000,
		},
		{
			name:      "fractional threshold",
			node:      node,
			threshold: 12.5,
			expected:  4_000,
		},
		{
			name:      "full threshold",
			node:      node,
			threshold: 100,
			expected:  32_000,
		},
		{
			name:      "missing allocatable cpu",
			node:      &v1.Node{},
			threshold: 25,
			expected:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := minCPUUsageMilliForThreshold(tc.node, tc.threshold); got != tc.expected {
				t.Fatalf("expected %vm, got %vm", tc.expected, got)
			}
		})
	}
}

func TestLowNodeUtilizationEvictionExpectation(t *testing.T) {
	tests := []struct {
		name     string
		actual   int
		min      int
		max      int
		expected bool
	}{
		{
			name:     "disabled metrics expects no evictions",
			actual:   0,
			min:      0,
			max:      0,
			expected: true,
		},
		{
			name:     "disabled metrics rejects evictions",
			actual:   1,
			min:      0,
			max:      0,
			expected: false,
		},
		{
			name:     "enabled metrics accepts one eviction",
			actual:   1,
			min:      1,
			max:      4,
			expected: true,
		},
		{
			name:     "enabled metrics accepts three evictions",
			actual:   3,
			min:      1,
			max:      4,
			expected: true,
		},
		{
			name:     "enabled metrics rejects no evictions",
			actual:   0,
			min:      1,
			max:      4,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := meetsLowNodeUtilizationEvictionExpectation(tc.actual, tc.min, tc.max); got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}
