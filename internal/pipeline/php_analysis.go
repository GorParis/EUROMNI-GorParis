package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

type PHPAnalysis struct {
	Platform     string
	PluginName   string
	Version      string
	SourceSHA256 string
	Hooks        []string
	HookDetails  []PHPHook
	Capabilities []string
	Features     []string
}

type PHPHook struct {
	Name    string
	Kind    string
	Handler string
}

func AnalyzePHPSource(source string) (*PHPAnalysis, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty PHP source", ErrPHPManifestInvalid)
	}

	sourceDigest := sha256.Sum256([]byte(source))
	analysis := &PHPAnalysis{SourceSHA256: hex.EncodeToString(sourceDigest[:])}
	if strings.Contains(trimmed, "woocommerce_") || strings.Contains(trimmed, "add_action('woocommerce_") || strings.Contains(trimmed, "add_filter('woocommerce_") {
		analysis.Platform = "woocommerce"
	} else if strings.Contains(trimmed, "registerHook") || strings.Contains(trimmed, "hookAction") || strings.Contains(trimmed, "Context::getContext") || strings.Contains(trimmed, "extends Module") || strings.Contains(trimmed, "extends PaymentModule") {
		analysis.Platform = "prestashop"
	} else {
		return nil, fmt.Errorf("%w: unsupported PHP platform", ErrPHPManifestInvalid)
	}

	pluginName := regexp.MustCompile(`(?im)\*\s*Plugin Name\s*:\s*([^\n*]+)`).FindStringSubmatch(trimmed)
	if len(pluginName) > 1 {
		analysis.PluginName = strings.TrimSpace(pluginName[1])
	}
	versionMatch := regexp.MustCompile(`(?im)\*\s*Version\s*:\s*([^\n*]+)`).FindStringSubmatch(trimmed)
	if len(versionMatch) > 1 {
		analysis.Version = strings.TrimSpace(versionMatch[1])
	}

	var matches [][]string
	if analysis.Platform == "prestashop" {
		registrationPattern := regexp.MustCompile(`(?is)\bregisterHook\s*\(\s*['"]([^'"]+)['"]`)
		matches = append(matches, registrationPattern.FindAllStringSubmatch(trimmed, -1)...)
		constantPattern := regexp.MustCompile(`(?is)\bconst\s+HOOKS\s*=\s*\[([^\]]*)\]`)
		stringPattern := regexp.MustCompile(`['"]([^'"]+)['"]`)
		for _, constant := range constantPattern.FindAllStringSubmatch(trimmed, -1) {
			if len(constant) > 1 {
				for _, hook := range stringPattern.FindAllStringSubmatch(constant[1], -1) {
					matches = append(matches, hook)
				}
			}
		}
	} else {
		hookPattern := regexp.MustCompile(`(?is)\b(?:add_action|add_filter)\s*\(\s*['"]([^'"]+)['"]`)
		matches = hookPattern.FindAllStringSubmatch(trimmed, -1)
	}
	seenHooks := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		hookName := strings.TrimSpace(match[1])
		if hookName == "" {
			continue
		}
		if _, exists := seenHooks[hookName]; exists {
			continue
		}
		seenHooks[hookName] = struct{}{}
		analysis.Hooks = append(analysis.Hooks, hookName)
	}
	if len(analysis.Hooks) == 0 {
		return nil, fmt.Errorf("%w: no recognized WordPress or platform hooks", ErrPHPManifestInvalid)
	}
	if analysis.Platform == "prestashop" {
		analysis.HookDetails = describePrestaShopHooks(trimmed, analysis.Hooks)
		if strings.Contains(trimmed, "Product::getNewProducts") &&
			strings.Contains(trimmed, "'NEW_PRODUCTS_NBR'") &&
			strings.Contains(trimmed, "'PS_NB_DAYS_NEW_PRODUCT'") {
			analysis.Features = append(analysis.Features, "prestashop.catalog.new_products")
		}
	}

	capabilitySet := map[string]struct{}{}
	for _, hook := range analysis.Hooks {
		normalizedHook := strings.ToLower(hook)
		switch {
		case strings.Contains(normalizedHook, "cart") || strings.Contains(normalizedHook, "calculate_totals") || strings.Contains(normalizedHook, "shipping"):
			capabilitySet["cart"] = struct{}{}
		case strings.Contains(normalizedHook, "product") || strings.Contains(normalizedHook, "catalog") || strings.Contains(normalizedHook, "attribute"):
			capabilitySet["catalog"] = struct{}{}
		case strings.Contains(normalizedHook, "order") || strings.Contains(normalizedHook, "checkout") || strings.Contains(normalizedHook, "payment") || strings.Contains(normalizedHook, "gateway") || strings.Contains(normalizedHook, "refund"):
			capabilitySet["order"] = struct{}{}
		case strings.Contains(normalizedHook, "customer") || strings.Contains(normalizedHook, "account") || strings.Contains(normalizedHook, "login"):
			capabilitySet["customer"] = struct{}{}
		}
	}
	if strings.Contains(trimmed, "$_POST") || strings.Contains(trimmed, "$_GET") {
		capabilitySet["customer"] = struct{}{}
	}
	for capability := range capabilitySet {
		analysis.Capabilities = append(analysis.Capabilities, capability)
	}
	if len(analysis.Capabilities) == 0 {
		analysis.Capabilities = []string{"catalog"}
	}
	return analysis, nil
}

func describePrestaShopHooks(source string, hooks []string) []PHPHook {
	methodPattern := regexp.MustCompile(`(?im)\bfunction\s+hook([A-Z][A-Za-z0-9_]*)\s*\(`)
	widgetHandler := regexp.MustCompile(`(?is)\bimplements\s+[^{}]*\bWidgetInterface\b`).MatchString(source) &&
		regexp.MustCompile(`(?im)\bfunction\s+renderWidget\s*\(`).MatchString(source)
	handlers := make(map[string]string)
	for _, match := range methodPattern.FindAllStringSubmatch(source, -1) {
		if len(match) < 2 {
			continue
		}
		methodSuffix := match[1]
		hookName := strings.ToLower(methodSuffix[:1]) + methodSuffix[1:]
		handlers[hookName] = "hook" + methodSuffix
	}

	details := make([]PHPHook, 0, len(hooks))
	for _, hook := range hooks {
		kind := "other"
		switch {
		case strings.HasPrefix(hook, "action"):
			kind = "action"
		case strings.HasPrefix(hook, "display"):
			kind = "display"
		}
		handler := handlers[hook]
		if handler == "" && kind == "display" && widgetHandler {
			handler = "renderWidget"
		}
		details = append(details, PHPHook{Name: hook, Kind: kind, Handler: handler})
	}
	return details
}

func hasPHPFeature(features []string, feature string) bool {
	for _, candidate := range features {
		if candidate == feature {
			return true
		}
	}
	return false
}

func hasPHPHookDetail(details []PHPHook, want PHPHook) bool {
	for _, detail := range details {
		if detail == want {
			return true
		}
	}
	return false
}
