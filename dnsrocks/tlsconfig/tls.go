/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tlsconfig

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/golang/glog"
)

func sessionTicketFromSeed(seed string) (key [32]byte) {
	h := sha256.New()
	h.Write([]byte(seed))
	copy(key[:], h.Sum(nil))
	return
}

func loadSessionTicketFromFile(config *tls.Config, seedfile string) error {
	seedReader, err := os.Open(seedfile)
	if err != nil {
		return fmt.Errorf("could not load seed file %q", seedfile)
	}
	defer seedReader.Close()
	seeds, err := loadSessionTicketKeys(seedReader)
	if err != nil {
		// If we cannot load the seeds, let's log it but not fail.
		return fmt.Errorf("could not load TLS seeds from %s", seedfile)
	}

	config.SetSessionTicketKeys(seeds)
	return nil
}

func loadSessionTicketKeys(reader io.Reader) (keys [][32]byte, err error) {
	var (
		t struct {
			Old     []string
			New     []string
			Current []string
		}
		data []byte
	)
	if data, err = io.ReadAll(reader); err != nil {
		return
	}
	if err = json.Unmarshal(data, &t); err != nil {
		return
	}
	if len(t.Current) == 0 || len(t.Old) == 0 || len(t.New) == 0 {
		err = fmt.Errorf("some of the seeds are missing")
		return
	}
	// closure to append session tickets to the list for each attributes of our
	// JSON struct.
	appendKeys := func(seeds []string) {
		for _, s := range seeds {
			keys = append(keys, sessionTicketFromSeed(s))
		}
	}
	appendKeys(t.Current)
	appendKeys(t.Old)
	appendKeys(t.New)
	return
}

func initSessionTicketKeys(ctx context.Context, config *tls.Config, keyConfig *SessionTicketKeysConfig) {
	if keyConfig.SeedFile == "" {
		glog.Infof("Skipping loading session ticket keys, no seed file provided.")
		return
	}
	err := loadSessionTicketFromFile(config, keyConfig.SeedFile)
	if err != nil {
		glog.Errorf(
			"Failed to load session tickets: %s. Skipping periodic reload ticker",
			err,
		)
	}
	if keyConfig.SeedFileReloadInterval > 0 {
		glog.Infof(
			"Setting ticker to reload seed file %s every %d seconds.",
			keyConfig.SeedFile,
			keyConfig.SeedFileReloadInterval,
		)
		ticker := time.NewTicker(
			time.Duration(keyConfig.SeedFileReloadInterval) * time.Second,
		)
		go func() {
			for {
				select {
				case <-ticker.C:
					err := loadSessionTicketFromFile(config, keyConfig.SeedFile)
					if err != nil {
						glog.Errorf(
							"Failed to load session tickets: %s",
							err,
						)
					}
				case <-ctx.Done():
					ticker.Stop()
					return
				}
			}
		}()
	} else {
		glog.Infof("Not reloading seed file")
	}
}

// InitTLSConfig loads keys and certs from a base TLSConfig into a new
// TLSConfig.  If configured, the TLSConfig will periodically refresh its
// session ticket keys for the lifetime of the context.
func InitTLSConfig(ctx context.Context, conf *TLSConfig) (*tls.Config, error) {
	var err error
	var certs []tls.Certificate
	var cert tls.Certificate
	cert, err = LoadTLSCertFromFile(conf)
	certs = []tls.Certificate{cert}
	if err != nil {
		return nil, err
	}
	config := tls.Config{
		Certificates: certs,
	}
	initSessionTicketKeys(ctx, &config, &conf.SessionTicketKeys)
	return &config, nil
}
