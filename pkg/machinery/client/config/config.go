// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v3"
)

// Config represents the configuration file.
type Config struct {
	Context  string              `yaml:"context"`
	Contexts map[string]*Context `yaml:"contexts"`
}

func (c *Config) upgrade() {
	for _, ctx := range c.Contexts {
		ctx.upgrade()
	}
}

// Context represents the set of credentials required to talk to a target.
type Context struct {
	DeprecatedTarget string   `yaml:"target,omitempty"` // Field deprecated in favor of Endpoints
	Endpoints        []string `yaml:"endpoints"`
	Nodes            []string `yaml:"nodes,omitempty"`
	CA               string   `yaml:"ca"`
	Crt              string   `yaml:"crt"`
	Key              string   `yaml:"key"`
}

func (c *Context) upgrade() {
	if c.DeprecatedTarget != "" {
		c.Endpoints = append(c.Endpoints, c.DeprecatedTarget)
		c.DeprecatedTarget = ""
	}
}

// Open reads the config and initializes a Config struct.
func Open(p string) (c *Config, err error) {
	if err = ensure(p); err != nil {
		return nil, err
	}

	var f *os.File

	f, err = os.Open(p)
	if err != nil {
		return
	}

	defer f.Close() //nolint: errcheck

	return ReadFrom(f)
}

// FromString returns a config from a string.
func FromString(p string) (c *Config, err error) {
	return ReadFrom(bytes.NewReader([]byte(p)))
}

// ReadFrom reads a config from io.Reader.
func ReadFrom(r io.Reader) (c *Config, err error) {
	c = &Config{}

	if err = yaml.NewDecoder(r).Decode(c); err != nil {
		return
	}

	c.upgrade()

	return
}

// Save writes the config to disk.
func (c *Config) Save(p string) (err error) {
	configBytes, err := yaml.Marshal(c)
	if err != nil {
		return
	}

	if err = os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}

	if err = ioutil.WriteFile(p, configBytes, 0o600); err != nil {
		return
	}

	return nil
}

// Merge in additional contexts from another Config.
//
// Current context is overridden from passed in config.
func (c *Config) Merge(cfg *Config) {
	for name, ctx := range cfg.Contexts {
		c.Contexts[name] = ctx
	}

	if cfg.Context != "" {
		c.Context = cfg.Context
	}
}

func ensure(filename string) (err error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		config := &Config{
			Context:  "",
			Contexts: map[string]*Context{},
		}

		return config.Save(filename)
	}

	return nil
}
