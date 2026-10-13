package pipeline

import (
	"encoding/xml"
	"fmt"
	"strings"
)

type MagentoPlugin struct {
	Subject string
	Name    string
	Type    string
}

type MagentoPreference struct {
	For  string
	Type string
}

type MagentoArgument struct {
	Subject string
	Name    string
	Values  []string
}

type MagentoAnalysis struct {
	ModuleName  string
	Events      []string
	Plugins     []MagentoPlugin
	Preferences []MagentoPreference
	Arguments   []MagentoArgument
}

func AnalyzeMagentoModule(moduleXML, eventsXML, diXML []byte) (*MagentoAnalysis, error) {
	var moduleDocument struct {
		Module struct {
			Name string `xml:"name,attr"`
		} `xml:"module"`
	}
	if err := xml.Unmarshal(moduleXML, &moduleDocument); err != nil {
		return nil, fmt.Errorf("invalid Magento module.xml: %w", err)
	}
	moduleName := strings.TrimSpace(moduleDocument.Module.Name)
	if moduleName == "" {
		return nil, fmt.Errorf("invalid Magento module.xml: module name is required")
	}
	analysis := &MagentoAnalysis{ModuleName: moduleName}

	if len(eventsXML) > 0 {
		var eventsDocument struct {
			Events []struct {
				Name string `xml:"name,attr"`
			} `xml:"event"`
		}
		if err := xml.Unmarshal(eventsXML, &eventsDocument); err != nil {
			return nil, fmt.Errorf("invalid Magento events.xml: %w", err)
		}
		seenEvents := map[string]struct{}{}
		for _, event := range eventsDocument.Events {
			name := strings.TrimSpace(event.Name)
			if name == "" {
				continue
			}
			if _, exists := seenEvents[name]; exists {
				continue
			}
			seenEvents[name] = struct{}{}
			analysis.Events = append(analysis.Events, name)
		}
	}

	if len(diXML) > 0 {
		var diDocument struct {
			Types []struct {
				Name    string `xml:"name,attr"`
				Plugins []struct {
					Name string `xml:"name,attr"`
					Type string `xml:"type,attr"`
				} `xml:"plugin"`
				Arguments []struct {
					Name  string `xml:"name,attr"`
					Items []struct {
						Value string `xml:",chardata"`
					} `xml:"arguments>argument>item"`
				} `xml:"arguments>argument"`
			} `xml:"type"`
			Preferences []struct {
				For  string `xml:"for,attr"`
				Type string `xml:"type,attr"`
			} `xml:"preference"`
		}
		if err := xml.Unmarshal(diXML, &diDocument); err != nil {
			return nil, fmt.Errorf("invalid Magento di.xml: %w", err)
		}
		for _, subject := range diDocument.Types {
			for _, plugin := range subject.Plugins {
				analysis.Plugins = append(analysis.Plugins, MagentoPlugin{
					Subject: strings.TrimSpace(subject.Name),
					Name:    strings.TrimSpace(plugin.Name),
					Type:    strings.TrimSpace(plugin.Type),
				})
			}
			for _, argument := range subject.Arguments {
				parsed := MagentoArgument{Subject: strings.TrimSpace(subject.Name), Name: strings.TrimSpace(argument.Name)}
				for _, item := range argument.Items {
					if value := strings.TrimSpace(item.Value); value != "" {
						parsed.Values = append(parsed.Values, value)
					}
				}
				analysis.Arguments = append(analysis.Arguments, parsed)
			}
		}
		for _, preference := range diDocument.Preferences {
			analysis.Preferences = append(analysis.Preferences, MagentoPreference{
				For:  strings.TrimSpace(preference.For),
				Type: strings.TrimSpace(preference.Type),
			})
		}
	}
	return analysis, nil
}
