package config

import (
	"testing"
	"time"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

const (
	globalLabelLimit            = 30
	globalLabelNameLengthLimit  = 200
	globalLabelValueLengthLimit = 200
)

var expectedConf = &Config{
	GlobalConfig: GlobalConfig{
		LabelLimit:            globalLabelLimit,
		LabelNameLengthLimit:  globalLabelNameLengthLimit,
		LabelValueLengthLimit: globalLabelValueLengthLimit,
		ExternalLabels:        model.LabelSet{"foo": "bar", "monitor": "codelab"},
	},
	AlertingConfig: AlertingConfig{
		AlertmanagerConfigs: AlertmanagerConfig{
			Scheme:           "https",
			Timeout:          model.Duration(10 * time.Second),
			APIVersion:       AlertmanagerAPIVersionV2,
			HTTPClientConfig: config.DefaultHTTPClientConfig,
			StaticConfigs: []*TargetConfig{
				{
					Targets: []model.LabelSet{
						{model.AddressLabel: "1.2.3.4:9093"},
						{model.AddressLabel: "1.2.3.5:9093"},
						{model.AddressLabel: "1.2.3.6:9093"},
					},
					Source: "0",
				},
			},
		},
	},
}

func TestLoadConfig(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml")
	require.NoError(t, err)
	t.Logf("loadted config:\n%+v\n", c)
	for _, a := range c.AlertingConfig.AlertmanagerConfigs.StaticConfigs {
		t.Logf("target:\n%+v\n", a.Targets)
	}
}

var expectedErrors = []struct {
	filename string
	errMsg   string
}{
	{
		filename: "labelname.bad.yml",
		errMsg:   `"not$allowed" is not a valid label name`,
	},
	{
		filename: "labelname2.bad.yml",
		errMsg:   `"not:allowed" is not a valid label name`,
	},
	{
		filename: "labelvalue.bad.yml",
		errMsg:   `invalid value "\xff"`,
	},
	{
		filename: "empty_alert_relabel_config.bad.yml",
		errMsg:   "empty or null alert relabeling rule",
	},
	{
		filename: "empty_alertmanager_relabel_config.bad.yml",
		errMsg:   "empty or null Alertmanager target relabeling rule",
	},
}

func TestBadConfigs(t *testing.T) {
	for _, ee := range expectedErrors {
		_, err := LoadFile("testdata/" + ee.filename)
		require.Error(t, err, "%s", ee.filename)
		require.Contains(t, err.Error(), ee.errMsg,
			"expected error for %s to contain %q but  got: %s", ee.filename, ee.errMsg, err)
	}
}

func TestEmptyConfig(t *testing.T) {
	c, err := Load("")
	require.NoError(t, err)
	exp := DefaultConfig
	require.Equal(t, exp, *c)
}

func TestEmptyGlobalBlock(t *testing.T) {
	c, err := Load("global:\n")
	require.NoError(t, err)
	exp := DefaultConfig
	require.Equal(t, exp, *c)
}
