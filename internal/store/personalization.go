package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var personalizationCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

var ErrInvalidPersonalization = errors.New("invalid product personalization")

func ValidatePersonalizationOptions(options []PersonalizationOption) error {
	if len(options) > 10 {
		return errors.New("at most 10 personalization options are allowed")
	}
	codes := make(map[string]struct{}, len(options))
	for _, option := range options {
		if !personalizationCodePattern.MatchString(option.Code) ||
			strings.TrimSpace(option.Label) != option.Label || len(option.Label) == 0 || len(option.Label) > 100 ||
			len(option.Choices) == 0 || len(option.Choices) > 50 {
			return errors.New("each personalization option requires a valid code, label, and 1 to 50 choices")
		}
		if _, exists := codes[option.Code]; exists {
			return errors.New("personalization option codes must be unique")
		}
		codes[option.Code] = struct{}{}
		choices := make(map[string]struct{}, len(option.Choices))
		for _, choice := range option.Choices {
			if !personalizationCodePattern.MatchString(choice.Value) ||
				strings.TrimSpace(choice.Label) != choice.Label || len(choice.Label) == 0 || len(choice.Label) > 100 {
				return errors.New("each personalization choice requires a valid value and label")
			}
			if _, exists := choices[choice.Value]; exists {
				return errors.New("personalization choice values must be unique per option")
			}
			choices[choice.Value] = struct{}{}
		}
	}
	encoded, err := json.Marshal(options)
	if err != nil || len(encoded) > 16<<10 {
		return errors.New("personalization options must be smaller than 16 KiB")
	}
	return nil
}

func ValidatePersonalizationSelections(options []PersonalizationOption, submitted map[string]string) ([]PersonalizationSelection, string, string, error) {
	if err := ValidatePersonalizationOptions(options); err != nil {
		return nil, "", "", fmt.Errorf("%w: invalid product configuration", ErrInvalidPersonalization)
	}
	if len(submitted) > len(options) {
		return nil, "", "", ErrInvalidPersonalization
	}
	byCode := make(map[string]PersonalizationOption, len(options))
	for _, option := range options {
		byCode[option.Code] = option
	}
	for code := range submitted {
		if _, exists := byCode[code]; !exists {
			return nil, "", "", ErrInvalidPersonalization
		}
	}
	selections := make([]PersonalizationSelection, 0, len(submitted))
	canonicalValues := make(map[string]string, len(submitted))
	for _, option := range options {
		value, selected := submitted[option.Code]
		if !selected || value == "" {
			if option.Required {
				return nil, "", "", ErrInvalidPersonalization
			}
			continue
		}
		found := false
		for _, choice := range option.Choices {
			if choice.Value == value {
				selections = append(selections, PersonalizationSelection{
					Code: option.Code, Label: option.Label, Value: choice.Value, ChoiceLabel: choice.Label,
				})
				canonicalValues[option.Code] = choice.Value
				found = true
				break
			}
		}
		if !found {
			return nil, "", "", ErrInvalidPersonalization
		}
	}
	snapshot, err := json.Marshal(selections)
	if err != nil {
		return nil, "", "", err
	}
	hashInput, err := json.Marshal(canonicalValues)
	if err != nil {
		return nil, "", "", err
	}
	digest := sha256.Sum256(hashInput)
	return selections, string(snapshot), hex.EncodeToString(digest[:]), nil
}

func personalizationValues(snapshot []byte) (map[string]string, error) {
	values := make(map[string]string)
	if len(snapshot) == 0 || string(snapshot) == "null" || string(snapshot) == "[]" {
		return values, nil
	}
	var selections []PersonalizationSelection
	if err := json.Unmarshal(snapshot, &selections); err != nil {
		// Older or empty cart snapshots can be represented as an empty object.
		var legacy map[string]any
		if legacyErr := json.Unmarshal(snapshot, &legacy); legacyErr != nil || len(legacy) != 0 {
			return nil, ErrInvalidPersonalization
		}
		return values, nil
	}
	for _, selection := range selections {
		if selection.Code == "" || selection.Value == "" {
			return nil, ErrInvalidPersonalization
		}
		if _, exists := values[selection.Code]; exists {
			return nil, ErrInvalidPersonalization
		}
		values[selection.Code] = selection.Value
	}
	return values, nil
}

func decodePersonalizationOptions(raw []byte) ([]PersonalizationOption, error) {
	var options []PersonalizationOption
	if len(raw) == 0 || string(raw) == "null" {
		return []PersonalizationOption{}, nil
	}
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil, err
	}
	if options == nil {
		options = []PersonalizationOption{}
	}
	if err := ValidatePersonalizationOptions(options); err != nil {
		return nil, err
	}
	return options, nil
}

func clonePersonalizationOptions(options []PersonalizationOption) []PersonalizationOption {
	cloned := make([]PersonalizationOption, len(options))
	for i, option := range options {
		cloned[i] = option
		cloned[i].Choices = append([]PersonalizationChoice(nil), option.Choices...)
	}
	return cloned
}
