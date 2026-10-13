package store

import "encoding/json"

func unmarshalObject(raw []byte, dest *map[string]any) error {
	if len(raw) == 0 {
		*dest = map[string]any{}
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return err
	}
	if *dest == nil {
		*dest = map[string]any{}
	}
	return nil
}
