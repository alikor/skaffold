/*
Copyright 2019 The Skaffold Authors

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

package cache

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/build"
	skafconfig "github.com/GoogleContainerTools/skaffold/pkg/skaffold/config"
	"github.com/docker/docker/api/types"

	"github.com/GoogleContainerTools/skaffold/cmd/skaffold/app/cmd/config"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/constants"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/docker"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/util"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// ArtifactCache is a map of [artifact dependencies hash : ImageDetails]
type ArtifactCache map[string]ImageDetails

// Cache holds any data necessary for accessing the cache
type Cache struct {
	artifactCache ArtifactCache
	client        docker.LocalDaemon
	builder       build.Builder
	imageList     []types.ImageSummary
	cacheFile     string
	useCache      bool
	needsPush     bool
	localCluster  bool
}

var (
	// For testing
	localCluster    = config.GetLocalCluster
	remoteDigest    = docker.RemoteDigest
	newDockerCilent = docker.NewAPIClient
	noCache         = &Cache{}
)

// NewCache returns the current state of the cache
func NewCache(ctx context.Context, builder build.Builder, opts *skafconfig.SkaffoldOptions, needsPush bool) *Cache {
	if !opts.CacheArtifacts {
		return noCache
	}
	cf, err := resolveCacheFile(opts.CacheFile)
	if err != nil {
		logrus.Warnf("Error resolving cache file, not using skaffold cache: %v", err)
		return noCache
	}
	cache, err := retrieveArtifactCache(cf)
	if err != nil {
		logrus.Warnf("Error retrieving artifact cache, not using skaffold cache: %v", err)
		return noCache
	}
	client, err := newDockerCilent()
	if err != nil {
		logrus.Warnf("Error retrieving local daemon client; local daemon will not be used as a cache: %v", err)
	}
	var imageList []types.ImageSummary
	if client != nil {
		imageList, err = client.ImageList(ctx, types.ImageListOptions{})
		if err != nil {
			logrus.Warn("Unable to get list of images from local docker daemon, won't be checked for cache.")
		}
	}

	lc, err := localCluster()
	if err != nil {
		logrus.Warn("Unable to determine if using a local cluster, cache may not work.")
	}
	return &Cache{
		artifactCache: cache,
		cacheFile:     cf,
		useCache:      opts.CacheArtifacts,
		client:        client,
		builder:       builder,
		needsPush:     needsPush,
		imageList:     imageList,
		localCluster:  lc,
	}
}

// resolveCacheFile makes sure that either a passed in cache file or the default cache file exists
func resolveCacheFile(cacheFile string) (string, error) {
	if cacheFile != "" {
		return cacheFile, util.VerifyOrCreateFile(cacheFile)
	}
	home, err := homedir.Dir()
	if err != nil {
		return "", errors.Wrap(err, "retrieving home directory")
	}
	defaultFile := filepath.Join(home, constants.DefaultSkaffoldDir, constants.DefaultCacheFile)
	return defaultFile, util.VerifyOrCreateFile(defaultFile)
}

func retrieveArtifactCache(cacheFile string) (ArtifactCache, error) {
	cache := ArtifactCache{}
	contents, err := ioutil.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(contents, &cache); err != nil {
		return nil, err
	}
	return cache, nil
}
