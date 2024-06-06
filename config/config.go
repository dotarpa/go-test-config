package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	prom_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/sigv4"
	"github.com/prometheus/prometheus/model/relabel"
	"gopkg.in/yaml.v2"
)

var (
	DefaultConfig = Config{
		GlobalConfig: DefaultGlobalConfig,
	}

	DefaultGlobalConfig      = GlobalConfig{}
	DefaultAlertmangerConfig = AlertmanagerConfig{
		Scheme:           "http",
		Timeout:          model.Duration(10 * time.Second),
		APIVersion:       AlertmanagerAPIVersionV2,
		HTTPClientConfig: prom_config.DefaultHTTPClientConfig,
	}
)

type Config struct {
	GlobalConfig   GlobalConfig   `yaml:"global" json:"global"`
	AlertingConfig AlertingConfig `yaml:"alerting"`
}

func (c Config) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}

	return string(b)
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.GlobalConfig.isZero() {
		c.GlobalConfig = DefaultGlobalConfig
	}

	return nil
}

func (c *GlobalConfig) isZero() bool {
	return len(c.ExternalLabels) == 0
}

type GlobalConfig struct {
	ExternalLabels        model.LabelSet `yaml:"external_labels,omitempty"`
	LabelLimit            uint           `yaml:"label_limit,omitempty"`
	LabelNameLengthLimit  uint           `yaml:"label_name_length_limit,omitempty"`
	LabelValueLengthLimit uint           `yaml:"label_value_length_limit,omitempty"`
}

func (c *GlobalConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	gc := &GlobalConfig{}
	type plain GlobalConfig
	if err := unmarshal((*plain)(gc)); err != nil {
		return err
	}

	if err := gc.ExternalLabels.Validate(); err != nil {
		return err
	}

	return unmarshal((*plain)(c))
}

type AlertingConfig struct {
	AlertRelabelConfigs []*relabel.Config  `yaml:"alert_relabel_configs,omitempty"`
	AlertmanagerConfigs AlertmanagerConfig `yaml:"alertmanager,omitempty"`
}

func (c *AlertingConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = AlertingConfig{}
	type plain AlertingConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	for _, rlcfg := range c.AlertRelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null alert relabeling rule")
		}
	}

	return nil
}

type AlertmanagerAPIVersion string

const (
	AlertmanagerAPIVersionV1 AlertmanagerAPIVersion = "v1"
	AlertmanagerAPIVersionV2 AlertmanagerAPIVersion = "v2"
)

var SupportedAlertmanagerAPIVersions = []AlertmanagerAPIVersion{
	AlertmanagerAPIVersionV1, AlertmanagerAPIVersionV2,
}

func (v *AlertmanagerAPIVersion) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*v = AlertmanagerAPIVersion("")
	type plain AlertmanagerAPIVersion
	if err := unmarshal((*plain)(v)); err != nil {
		return err
	}

	for _, supportedVersion := range SupportedAlertmanagerAPIVersions {
		if *v == supportedVersion {
			return nil
		}
	}

	return fmt.Errorf("expected Alertmanager api version to be one of %v but got %v", SupportedAlertmanagerAPIVersions, *v)
}

type AlertmanagerConfig struct {
	StaticConfigs    []*TargetConfig              `yaml:"static_configs"`
	HTTPClientConfig prom_config.HTTPClientConfig `yaml:",inline"`
	SigV4Config      *sigv4.SigV4Config           `yaml:"sigv4,omitempty"`

	Scheme              string                 `yaml:"scheme,omitempty"`
	PathPrefix          string                 `yaml:"path_prefix,omitempty"`
	Timeout             model.Duration         `yaml:"timeout,omitempty"`
	APIVersion          AlertmanagerAPIVersion `yaml:"api_version"`
	RelabelConfigs      []*relabel.Config      `yaml:"relabel_configs,omitempty"`
	AlertRelabelConfigs []*relabel.Config      `yaml:"alert_relabel_configs,omitempty"`
}

func (c *AlertmanagerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultAlertmangerConfig
	type plain AlertmanagerConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	httpClientConfigAuthEnabled := c.HTTPClientConfig.BasicAuth != nil ||
		c.HTTPClientConfig.Authorization != nil || c.HTTPClientConfig.OAuth2 != nil

	if httpClientConfigAuthEnabled && c.SigV4Config != nil {
		return fmt.Errorf("at most one of basic_auth, authorization, oauth2, & sigv4 must be configured")
	}

	if len(c.RelabelConfigs) == 0 {
		if err := checkTargets(c.StaticConfigs); err != nil {
			return err
		}
	}

	for _, rlcfg := range c.RelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null Alertmanager target relabeling rule")
		}
	}

	for _, rlcfg := range c.AlertRelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null Alertmanager alert relabeling rule")
		}
	}

	return nil
}

func CheckTargetAddress(address model.LabelValue) error {
	if strings.Contains(string(address), "/") {
		return fmt.Errorf("%q is not a valid hostname", address)
	}

	return nil
}

type TargetConfig struct {
	Targets []model.LabelSet
	Labels  model.LabelSet
	Source  string
}

func (tc TargetConfig) String() string {
	return tc.Source
}

func (tc *TargetConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	t := struct {
		Targets []string       `yaml:"targets"`
		Labels  model.LabelSet `yaml:"labels"`
	}{}

	if err := unmarshal(&t); err != nil {
		return err
	}

	tc.Targets = make([]model.LabelSet, 0, len(t.Targets))

	for _, target := range t.Targets {
		tc.Targets = append(tc.Targets, model.LabelSet{
			model.AddressLabel: model.LabelValue(target),
		})
	}

	tc.Labels = t.Labels

	return nil
}

func checkTargets(configs []*TargetConfig) error {
	for _, cfg := range configs {
		for _, t := range cfg.Targets {
			if err := CheckTargetAddress(t[model.AddressLabel]); err != nil {
				return err
			}
		}
	}
	return nil
}

func Load(s string) (*Config, error) {
	cfg := &Config{}
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func LoadFile(filename string) (*Config, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	cfg, err := Load(string(content))
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
