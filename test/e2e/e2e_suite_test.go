//go:build e2e
// +build e2e

/*
Copyright 2026.

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
	"fmt"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

// TestE2E runs the Godog BDD test suite against features in test/e2e/features.
func TestE2E(t *testing.T) {
	opts := godog.Options{
		Format:   "pretty",
		Paths:    []string{"features"},
		Output:   colors.Colored(os.Stdout),
		TestingT: t,
		Strict:   true,
	}

	suite := godog.TestSuite{
		Name:                 "k8s-sandbox-controller-e2e",
		ScenarioInitializer:  InitializeScenario,
		TestSuiteInitializer: InitializeTestSuite,
		Options:              &opts,
	}

	if status := suite.Run(); status != 0 {
		t.Fatalf("Godog e2e test suite failed with exit code: %d", status)
	}
}

// InitializeTestSuite handles global test suite lifecycle.
func InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		fmt.Println("Starting Godog E2E test suite for k8s-sandbox-controller")
	})
	ctx.AfterSuite(func() {
		fmt.Println("Completed Godog E2E test suite")
	})
}
