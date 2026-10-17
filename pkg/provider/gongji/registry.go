package gongji

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

const (
	AvailabilitySchedulable = "schedulable"
	AvailabilityMaintenance = "maintenance"
	AvailabilityPaused      = "paused"
	AvailabilityRetired     = "retired"

	ComplianceDomestic = "domestic"
	ComplianceOverseas = "overseas"

	ProviderIDGongji = "gongji"
)

var logicalAzPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// RegionBinding maps one logical AZ to a single Gongji physical region and SKU.
type RegionBinding struct {
	LogicalAzID      string `yaml:"logical_az_id" json:"logicalAzId"`
	DisplayName      string `yaml:"display_name" json:"displayName"`
	ComplianceDomain string `yaml:"compliance_domain" json:"complianceDomain"`
	Availability     string `yaml:"availability" json:"availability"`
	ProviderID       string `yaml:"provider_id" json:"providerId"`
	PhysicalRegionID string `yaml:"physical_region_id" json:"physicalRegionId"`
	ProviderSkuID    string `yaml:"provider_sku_id" json:"providerSkuId"`
	GPUCount         int    `yaml:"gpu_count" json:"gpuCount"`
}

type regionFile struct {
	Version int              `yaml:"version"`
	Regions []regionFileItem `yaml:"regions"`
}

type regionFileItem struct {
	LogicalAzID      string             `yaml:"logical_az_id"`
	DisplayName      string             `yaml:"display_name"`
	ComplianceDomain string             `yaml:"compliance_domain"`
	Availability     string             `yaml:"availability"`
	Providers        []regionFileVendor `yaml:"providers"`
}

type regionFileVendor struct {
	ProviderID       string `yaml:"provider_id"`
	PhysicalRegionID string `yaml:"physical_region_id"`
	ProviderSkuID    string `yaml:"provider_sku_id"`
	GPUCount         int    `yaml:"gpu_count"`
}

// LoadRegionFile parses and validates a logical AZ registry.
// A missing file is not an error; the caller treats that as an empty registry.
func LoadRegionFile(path string) ([]RegionBinding, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read region registry %s: %w", path, err)
	}
	return ParseRegionFile(data)
}

// ParseRegionFile validates registry YAML.
func ParseRegionFile(data []byte) ([]RegionBinding, error) {
	var file regionFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse region registry: %w", err)
	}
	if len(file.Regions) == 0 {
		return nil, fmt.Errorf("region registry has no regions")
	}
	out := make([]RegionBinding, 0, len(file.Regions))
	seenAZ := map[string]struct{}{}
	seenBind := map[string]struct{}{}
	for i, region := range file.Regions {
		if !logicalAzPattern.MatchString(region.LogicalAzID) {
			return nil, fmt.Errorf("regions[%d].logical_az_id %q is invalid", i, region.LogicalAzID)
		}
		if _, ok := seenAZ[region.LogicalAzID]; ok {
			return nil, fmt.Errorf("duplicate logical_az_id %s", region.LogicalAzID)
		}
		seenAZ[region.LogicalAzID] = struct{}{}
		if region.DisplayName == "" {
			return nil, fmt.Errorf("regions[%d] display_name is empty", i)
		}
		if region.ComplianceDomain != ComplianceDomestic && region.ComplianceDomain != ComplianceOverseas {
			return nil, fmt.Errorf("regions[%d] compliance_domain %q is invalid", i, region.ComplianceDomain)
		}
		availability := region.Availability
		if availability == "" {
			availability = AvailabilitySchedulable
		}
		if !validAvailability(availability) {
			return nil, fmt.Errorf("regions[%d] availability %q is invalid", i, availability)
		}
		if len(region.Providers) != 1 {
			return nil, fmt.Errorf("regions[%d] must bind exactly one provider in P0", i)
		}
		vendor := region.Providers[0]
		if vendor.ProviderID != ProviderIDGongji {
			return nil, fmt.Errorf("regions[%d] provider_id %q is not registered", i, vendor.ProviderID)
		}
		if vendor.PhysicalRegionID == "" || vendor.ProviderSkuID == "" || vendor.GPUCount <= 0 {
			return nil, fmt.Errorf("regions[%d] physical region, sku, and gpu_count are required", i)
		}
		key := bindingKey(vendor.ProviderID, vendor.PhysicalRegionID, vendor.ProviderSkuID, vendor.GPUCount)
		if _, ok := seenBind[key]; ok {
			return nil, fmt.Errorf("duplicate provider binding %s", key)
		}
		seenBind[key] = struct{}{}
		out = append(out, RegionBinding{
			LogicalAzID:      region.LogicalAzID,
			DisplayName:      region.DisplayName,
			ComplianceDomain: region.ComplianceDomain,
			Availability:     availability,
			ProviderID:       vendor.ProviderID,
			PhysicalRegionID: vendor.PhysicalRegionID,
			ProviderSkuID:    vendor.ProviderSkuID,
			GPUCount:         vendor.GPUCount,
		})
	}
	return out, nil
}

func validAvailability(v string) bool {
	switch v {
	case AvailabilitySchedulable, AvailabilityMaintenance, AvailabilityPaused, AvailabilityRetired:
		return true
	default:
		return false
	}
}

func bindingKey(provider, region, sku string, gpuCount int) string {
	return fmt.Sprintf("%s|%s|%s|%d", provider, region, sku, gpuCount)
}

func offerKey(region, gpuName string, gpuCount int) string {
	return fmt.Sprintf("%s|%s|%d", region, gpuName, gpuCount)
}
