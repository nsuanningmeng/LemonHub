package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// GetUserPerformanceModelGroups uses the current model memberships and the
// viewer's usable groups, matching the performance data shown alongside pricing.
func GetUserPerformanceModelGroups(userGroup string) map[string][]string {
	usableGroups := GetUserUsableGroups(userGroup)
	activeRatios := ratio_setting.GetGroupRatioCopy()
	modelGroups := make(map[string][]string)
	for _, pricing := range model.GetPricing() {
		for _, group := range pricing.EnableGroup {
			if group == "" || group == "auto" {
				continue
			}
			if _, ok := usableGroups[group]; !ok {
				continue
			}
			if _, ok := activeRatios[group]; !ok {
				continue
			}
			modelGroups[pricing.ModelName] = append(modelGroups[pricing.ModelName], group)
		}
	}
	return modelGroups
}
