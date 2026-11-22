package core

import (
	"fmt"
	"slices"
)

type ProfileID string

const (
	ProfileDefault    ProfileID = "default"
	ProfileHeadless   ProfileID = "headless"
	ProfileInspection ProfileID = "inspection"
)

type componentSelection struct {
	id   ComponentID
	slot ComponentSlot
}

type runtimeProfile struct {
	id         ProfileID
	components []componentSelection
	capability string
}

func selectRuntimeProfile(opts Options) (runtimeProfile, error) {
	id := opts.Profile
	// Inspection is the legacy opening option.
	if opts.Inspection {
		if id != "" && id != ProfileInspection {
			return runtimeProfile{}, fmt.Errorf("profile %q conflicts with inspection", id)
		}
		id = ProfileInspection
	}
	if id == "" {
		id = ProfileDefault
	}
	base := []componentSelection{{id: "storage", slot: StorageSlot}, {id: "startup-settings"}}
	var profile runtimeProfile
	switch id {
	case ProfileDefault, ProfileHeadless:
		profile = runtimeProfile{id: id, components: append(base,
			componentSelection{id: "execution-state"}, componentSelection{id: "provider"},
			componentSelection{id: "tools"}, componentSelection{id: "runner"}, componentSelection{id: "application"},
		), capability: "execute"}
	case ProfileInspection:
		profile = runtimeProfile{id: id, components: append(base, componentSelection{id: "inspection"}), capability: "inspect-records"}
	default:
		return runtimeProfile{}, fmt.Errorf("unknown composition profile %q", id)
	}
	for slot, replacement := range opts.Selections {
		index := slices.IndexFunc(profile.components, func(selection componentSelection) bool { return slot != "" && selection.slot == slot })
		if index == -1 || replacement == "" {
			return runtimeProfile{}, fmt.Errorf("profile %q has no valid replacement for slot %q", id, slot)
		}
		profile.components[index].id = replacement
	}
	return profile, nil
}
