package sandbox

import (
	"fmt"
	"strings"
)

type NetworkAction string

const (
	NetworkActionAllow NetworkAction = "allow"
	NetworkActionDeny  NetworkAction = "deny"
)

type NetworkRule struct {
	Action NetworkAction
	Target string
}

func (rule NetworkRule) Validate() error {
	if rule.Action != NetworkActionAllow && rule.Action != NetworkActionDeny {
		return fmt.Errorf("invalid sandbox network action %q", rule.Action)
	}
	if strings.TrimSpace(rule.Target) == "" {
		return fmt.Errorf("sandbox network target is required")
	}
	return nil
}

type NetworkPolicy struct {
	DefaultAction NetworkAction
	Rules         []NetworkRule
}

func (policy NetworkPolicy) Validate() error {
	if policy.DefaultAction != NetworkActionAllow && policy.DefaultAction != NetworkActionDeny {
		return fmt.Errorf("invalid sandbox default network action %q", policy.DefaultAction)
	}
	for index, rule := range policy.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("invalid sandbox network rule %d: %w", index, err)
		}
	}
	return nil
}
