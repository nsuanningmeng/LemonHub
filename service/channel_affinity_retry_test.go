package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffinityFallbackStartsAtHighestPriorityWithinRetryBudget(t *testing.T) {
	// Restore an existing cache only after the DB fixture restores its connection.
	t.Cleanup(func() {
		if common.MemoryCacheEnabled && model.DB.Migrator().HasTable(&model.Ability{}) {
			require.NoError(t, model.InitChannelCache())
		}
	})
	setupChannelSelectTestDB(t)
	previousAuto := setting.AutoGroups2JsonString()
	previousUsable := setting.UserUsableGroups2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(previousAuto))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		// Do not leave fixture channels in the global memory cache.
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Channel{}).Error)
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Ability{}).Error)
		common.MemoryCacheEnabled = true
		require.NoError(t, model.InitChannelCache())
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["ga","gb"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","ga":"A","gb":"B"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"ga":1,"gb":1}`))
	for _, spec := range []struct {
		id       int
		group    string
		priority int64
	}{{11, "ga", 3}, {12, "ga", 2}, {13, "ga", 1}, {21, "gb", 3}, {22, "gb", 2}} {
		addChannelWithAbility(t, spec.id, spec.group, "affinity-model", spec.priority)
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", spec.id).Update("priority", spec.priority).Error)
	}
	for _, cached := range []bool{false, true} {
		for _, tc := range []struct {
			name                    string
			groups                  []string
			affinity, strict, cross bool
			retries                 int
			want                    []int
		}{
			{name: "ordinary", groups: []string{"ga"}, retries: 2, want: []int{11, 12, 13}},
			{name: "affinity single", groups: []string{"ga"}, affinity: true, retries: 2, want: []int{12, 11, 12}},
			{name: "one retry", groups: []string{"ga"}, affinity: true, retries: 1, want: []int{12, 11}},
			{name: "no budget", groups: []string{"ga"}, affinity: true, retries: 0, want: []int{12}},
			{name: "strict", groups: []string{"ga"}, affinity: true, strict: true, retries: 2, want: []int{12}},
			{name: "auto no cross", groups: []string{"auto"}, affinity: true, retries: 1, want: []int{12, 11}},
			{name: "auto cross", groups: []string{"auto"}, affinity: true, cross: true, retries: 1, want: []int{12, 11, 21, 22}},
			{name: "explicit groups", groups: []string{"ga", "gb"}, affinity: true, retries: 1, want: []int{12, 11, 21, 22}},
			{name: "skip empty resets offset", groups: []string{"empty", "ga"}, affinity: true, retries: 1, want: []int{12, 11, 12}},
		} {
			t.Run(fmt.Sprintf("cache=%t/%s", cached, tc.name), func(t *testing.T) {
				common.MemoryCacheEnabled = cached
				require.NoError(t, model.InitChannelCache())
				common.RetryTimes = tc.retries
				ctx := newCtxWithGroups("default", tc.groups)
				common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, tc.cross)
				param := &RetryParam{Ctx: ctx, TokenGroup: tc.groups[0], ModelName: "affinity-model", Retry: common.GetPointer(0)}
				var got []int
				if tc.affinity {
					setChannelAffinityContext(ctx, channelAffinityMeta{SkipRetry: tc.strict})
					MarkChannelAffinityUsed(ctx, "ga", 12)
					got = append(got, 12)
					param.IncreaseRetry()
				}
				if !ShouldSkipRetryAfterChannelAffinityFailure(ctx) {
					for ; param.GetRetry() <= common.RetryTimes; param.IncreaseRetry() {
						channel, _, err := CacheGetRandomSatisfiedChannel(param)
						require.NoError(t, err)
						if channel == nil {
							break
						}
						got = append(got, channel.Id)
						require.LessOrEqual(t, len(got), len(tc.want), "retry budget must not grow")
					}
				}
				assert.Equal(t, tc.want, got)
			})
		}
	}
}
