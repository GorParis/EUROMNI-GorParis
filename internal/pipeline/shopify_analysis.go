package pipeline

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type ShopifyExtension struct {
	Name            string
	Handle          string
	Type            string
	Targets         []string
	ExtensionPoints []string
}

type ShopifyAnalysis struct {
	APIVersion string
	Extensions []ShopifyExtension
}

type shopifyManifest struct {
	APIVersion      string   `toml:"api_version"`
	Type            string   `toml:"type"`
	Name            string   `toml:"name"`
	Handle          string   `toml:"handle"`
	ExtensionPoints []string `toml:"extension_points"`
	Targeting       []struct {
		Target string `toml:"target"`
	} `toml:"targeting"`
	Extensions []struct {
		Name      string `toml:"name"`
		Handle    string `toml:"handle"`
		Type      string `toml:"type"`
		Targeting []struct {
			Target string `toml:"target"`
		} `toml:"targeting"`
	} `toml:"extensions"`
}

func AnalyzeShopifyManifest(data []byte) (*ShopifyAnalysis, error) {
	var manifest shopifyManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid Shopify extension manifest: %w", err)
	}

	analysis := &ShopifyAnalysis{APIVersion: strings.TrimSpace(manifest.APIVersion)}
	for _, extension := range manifest.Extensions {
		parsed := ShopifyExtension{
			Name:   strings.TrimSpace(extension.Name),
			Handle: strings.TrimSpace(extension.Handle),
			Type:   strings.TrimSpace(extension.Type),
		}
		for _, targeting := range extension.Targeting {
			if target := strings.TrimSpace(targeting.Target); target != "" {
				parsed.Targets = append(parsed.Targets, target)
			}
		}
		analysis.Extensions = append(analysis.Extensions, parsed)
	}

	if manifest.Type != "" || manifest.Name != "" {
		parsed := ShopifyExtension{
			Name:            strings.TrimSpace(manifest.Name),
			Handle:          strings.TrimSpace(manifest.Handle),
			Type:            strings.TrimSpace(manifest.Type),
			ExtensionPoints: append([]string(nil), manifest.ExtensionPoints...),
		}
		for _, targeting := range manifest.Targeting {
			if target := strings.TrimSpace(targeting.Target); target != "" {
				parsed.Targets = append(parsed.Targets, target)
			}
		}
		analysis.Extensions = append(analysis.Extensions, parsed)
	}

	if len(analysis.Extensions) == 0 {
		return nil, fmt.Errorf("invalid Shopify extension manifest: no extensions found")
	}
	for _, extension := range analysis.Extensions {
		if extension.Name == "" || extension.Type == "" {
			return nil, fmt.Errorf("invalid Shopify extension manifest: extension name and type are required")
		}
		if len(extension.Targets) == 0 && len(extension.ExtensionPoints) == 0 {
			return nil, fmt.Errorf("invalid Shopify extension manifest: extension %q has no target", extension.Name)
		}
	}
	return analysis, nil
}
