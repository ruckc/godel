// Copyright 2016 Palantir Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dist_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"testing"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/godel/pkg/osarch"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/distgo/dister"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/palantir/distgo/dockerbuilder"
)

const (
	testMain = `package main

import "fmt"

var testVersionVar = "defaultVersion"

func main() {
	fmt.Println(testVersionVar)
}
`
)

func TestDist(t *testing.T) {
	tmp, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	defaultDisterCfg, err := dister.DefaultConfig()
	require.NoError(t, err)

	for i, tc := range []struct {
		name            string
		projectCfg      distgo.ProjectConfig
		preDistAction   func(projectDir string, projectCfg distgo.ProjectConfig)
		wantErrorRegexp string
		validate        func(caseNum int, name, projectDir string)
	}{
		{
			"default dist is os-arch-bin",
			distgo.ProjectConfig{},
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", fmt.Sprintf("foo-0.1.0-%s.tgz", osarch.Current().String())))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
			},
		},
		{
			"runs custom dist script",
			distgo.ProjectConfig{
				ProductDefaults: distgo.ProductConfig{
					Dist: &distgo.DistConfig{
						Disters: &distgo.DistersConfig{
							dister.OSArchBinDistTypeName: {
								Type:   defaultDisterCfg.Type,
								Config: defaultDisterCfg.Config,
								Script: stringPtr(`#!/usr/bin/env bash
touch $DIST_DIR/test-file.txt`),
							},
						},
					},
				},
			},
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "test-file.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
			},
		},
		{
			"custom dist script inherits process environment variables",
			distgo.ProjectConfig{
				ProductDefaults: distgo.ProductConfig{
					Dist: &distgo.DistConfig{
						Disters: &distgo.DistersConfig{
							dister.OSArchBinDistTypeName: {
								Type:   defaultDisterCfg.Type,
								Config: defaultDisterCfg.Config,
								Script: stringPtr(`#!/usr/bin/env bash
touch $DIST_DIR/$DIST_TEST_KEY.txt`),
							},
						},
					},
				},
			},
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
				err := os.Setenv("DIST_TEST_KEY", "distTestVal")
				require.NoError(t, err)
			},
			"",
			func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "distTestVal.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
				err = os.Unsetenv("DIST_TEST_KEY")
				require.NoError(t, err)
			},
		},
		{
			"custom dist script uses script includes",
			distgo.ProjectConfig{
				ScriptIncludes: `touch $DIST_DIR/foo.txt
helper_func() {
	touch $DIST_DIR/baz.txt
}`,
				ProductDefaults: distgo.ProductConfig{
					Dist: &distgo.DistConfig{
						Disters: &distgo.DistersConfig{
							dister.OSArchBinDistTypeName: {
								Type:   defaultDisterCfg.Type,
								Config: defaultDisterCfg.Config,
								Script: stringPtr(`#!/usr/bin/env bash
touch $DIST_DIR/$VERSION
helper_func`),
							},
						},
					},
				},
			},
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "baz.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "0.1.0"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
			},
		},
		{
			"script includes not executed if custom script not specified",
			distgo.ProjectConfig{
				ScriptIncludes: `touch $DIST_DIR/foo.txt
helper_func() {
	touch $DIST_DIR/baz.txt
}`,
			},
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			func(caseNum int, name, projectDir string) {
				_, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "foo.txt"))
				assert.True(t, os.IsNotExist(err), "Case %d: %s", caseNum, name)
			},
		},
		{
			"dependent products and dists are available",
			distgo.ProjectConfig{
				Products: map[distgo.ProductID]distgo.ProductConfig{
					"foo": {
						Build: &distgo.BuildConfig{
							MainPkg: stringPtr("foo"),
						},
						Dist: &distgo.DistConfig{
							Disters: &distgo.DistersConfig{
								dister.OSArchBinDistTypeName: {
									Type:   defaultDisterCfg.Type,
									Config: defaultDisterCfg.Config,
									Script: stringPtr(`#!/usr/bin/env bash
echo $DEP_PRODUCT_ID_COUNT $DEP_PRODUCT_ID_0 > $DIST_DIR/dep-product-ids.txt
echo $DEP_PRODUCT_ID_0_BUILD_DIR > $DIST_DIR/bar-build-dir.txt
echo $DEP_PRODUCT_ID_0_DIST_ID_0_DIST_DIR > $DIST_DIR/bar-dist-dir.txt
echo $DEP_PRODUCT_ID_0_DIST_ID_0_DIST_ARTIFACT_0 > $DIST_DIR/bar-dist-artifacts.txt
`),
								},
							},
						},
						Dependencies: &[]distgo.ProductID{
							"bar",
						},
					},
					"bar": {
						Build: &distgo.BuildConfig{
							MainPkg: stringPtr("bar"),
						},
						Dist: &distgo.DistConfig{
							Disters: &distgo.DistersConfig{
								dister.OSArchBinDistTypeName: {
									Type:   defaultDisterCfg.Type,
									Config: defaultDisterCfg.Config,
								},
							},
						},
					},
				},
			},
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				_, err := gofiles.Write(projectDir, []gofiles.GoFileSpec{
					{
						RelPath: "bar/main.go",
						Src: `package main

func main() {}
`,
					},
				})
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Add bar")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			func(caseNum int, name, projectDir string) {
				bytes, err := ioutil.ReadFile(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "dep-product-ids.txt"))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				assert.Equal(t, "1 bar\n", string(bytes), "Case %d: %s", caseNum, name)

				bytes, err = ioutil.ReadFile(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "bar-build-dir.txt"))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				assert.Equal(t, fmt.Sprintf("%s\n", path.Join(projectDir, "out", "build", "bar", "0.1.0")), string(bytes), "Case %d: %s", caseNum, name)

				bytes, err = ioutil.ReadFile(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "bar-dist-artifacts.txt"))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				assert.Equal(t, fmt.Sprintf("bar-0.1.0-%v.tgz\n", osarch.Current()), string(bytes), "Case %d: %s", caseNum, name)

				bytes, err = ioutil.ReadFile(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "bar-dist-dir.txt"))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				assert.Equal(t, fmt.Sprintf("%s\n", path.Join(projectDir, "out", "dist", "bar", "0.1.0", "os-arch-bin")), string(bytes), "Case %d: %s", caseNum, name)
			},
		},
	} {
		projectDir, err := ioutil.TempDir(tmp, "")
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		gittest.InitGitDir(t, projectDir)
		err = os.MkdirAll(path.Join(projectDir, "foo"), 0755)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		err = ioutil.WriteFile(path.Join(projectDir, "foo", "main.go"), []byte(testMain), 0644)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		gittest.CommitAllFiles(t, projectDir, "Commit")

		if tc.preDistAction != nil {
			tc.preDistAction(projectDir, tc.projectCfg)
		}

		disterFactory, err := dister.NewDisterFactory()
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		defaultDistCfg, err := dister.DefaultConfig()
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		dockerBuilderFactory, err := dockerbuilder.NewDockerBuilderFactory()
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		projectParam, err := tc.projectCfg.ToParam(projectDir, disterFactory, defaultDistCfg, dockerBuilderFactory)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		projectInfo, err := projectParam.ProjectInfo(projectDir)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		err = dist.Products(projectInfo, projectParam, nil, false, ioutil.Discard)
		if tc.wantErrorRegexp == "" {
			require.NoError(t, err, "Case %d: %s", i, tc.name)
		} else {
			require.Error(t, err, fmt.Sprintf("Case %d: %s", i, tc.name))
			assert.Regexp(t, regexp.MustCompile(tc.wantErrorRegexp), err.Error(), "Case %d: %s", i, tc.name)
		}

		if tc.validate != nil {
			tc.validate(i, tc.name, projectDir)
		}
	}
}

func stringPtr(in string) *string {
	return &in
}
