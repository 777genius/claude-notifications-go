package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	configtemplate "github.com/777genius/agent-notifications/config"
)

// AssetContext is separate from resolver inputs. LookupEnv must be injected;
// no ambient environment is read when constructing an effective Config.
type AssetContext struct {
	PluginRoot string
	LookupEnv  func(string) (string, bool)
}

// Effective constructs an independent, expanded runtime value using historical
// defaults and tri-state semantics. Never marshal its result back to storage.
func (d Document) Effective(assets AssetContext) (*Config, error) {
	c := buildDefaultConfig("${CLAUDE_PLUGIN_ROOT}")
	defaults := make(map[string]StatusInfo, len(c.Statuses))
	for k, v := range c.Statuses {
		defaults[k] = v
	}
	if err := decodeTyped(d.original, c); err != nil {
		return nil, &Error{Code: ConfigInvalid}
	}
	if err := mergeStatusOverrides(d.original, c, defaults); err != nil {
		return nil, &Error{Code: ConfigInvalid}
	}
	if c.Statuses == nil {
		c.Statuses = defaults
	}
	expand := func(s string) string {
		return os.Expand(s, func(k string) string {
			if k == "CLAUDE_PLUGIN_ROOT" {
				if assets.PluginRoot != "" {
					return assets.PluginRoot
				}
				if assets.LookupEnv != nil {
					if v, ok := assets.LookupEnv(k); ok && !isUnresolvedPluginRoot(v) {
						return v
					}
				}
				return "."
			}
			if assets.LookupEnv != nil {
				if v, ok := assets.LookupEnv(k); ok {
					return v
				}
			}
			return ""
		})
	}
	expandPathValue := func(s string) string {
		v := expand(s)
		if v == "" {
			return ""
		}
		return filepath.Clean(v)
	}
	c.Notifications.Desktop.AppIcon = expandPathValue(c.Notifications.Desktop.AppIcon)
	c.Notifications.Webhook.URL = expand(c.Notifications.Webhook.URL)
	for k, v := range c.Statuses {
		v.Sound = expandPathValue(v.Sound)
		c.Statuses[k] = v
	}
	// A nonempty literal root prevents applyDefaults consulting the environment.
	// Missing statuses were already present before decode and merged above.
	c.applyDefaults(".")
	if err := c.Validate(); err != nil {
		return nil, &Error{Code: ConfigInvalid}
	}
	return c, nil
}

// SeedDocument adds schema metadata to an independent copy of the exact shipped
// template. It is an internal capability only, not a runtime fallback cutover.
func SeedDocument(physicalPath string) (Document, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(configtemplate.Bytes(), &raw); err != nil {
		return Document{}, &Error{Code: ConfigInvalid}
	}
	raw["schemaVersion"] = json.RawMessage("1")
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return Document{}, &Error{Code: ConfigInvalid}
	}
	return ParseDocument(append(data, '\n'), physicalPath, false)
}
