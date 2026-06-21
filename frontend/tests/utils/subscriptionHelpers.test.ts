import {
  getCurrentPlanFeatures,
  shouldShowUpgradeOptions,
  getPaidPlans,
  matchesPlanName,
} from '../../src/utils/subscriptionHelpers'
import type {
  SubscriptionStatusResponse,
  ProductConfiguration,
} from '../../src/types'
import { SUBSCRIPTION_STATUS } from '../../src/types'

describe('subscriptionHelpers', () => {
  describe('matchesPlanName', () => {
    describe('Exact matches', () => {
      it('returns true for identical plan names', () => {
        expect(matchesPlanName('Professional', 'Professional')).toBe(true)
      })

      it('returns true for exact match with different casing', () => {
        expect(matchesPlanName('Professional', 'professional')).toBe(true)
        expect(matchesPlanName('PROFESSIONAL', 'professional')).toBe(true)
        expect(matchesPlanName('ProFessIoNal', 'PROFESSIONAL')).toBe(true)
      })

      it('returns true for exact match with leading/trailing spaces', () => {
        expect(matchesPlanName('Professional', 'Professional')).toBe(true)
      })
    })

    describe('Substring matches', () => {
      it('returns true when first name contains second name', () => {
        expect(matchesPlanName('VibeXP - Professional', 'Professional')).toBe(
          true
        )
        expect(matchesPlanName('VibeXP - Starter', 'Starter')).toBe(true)
        expect(matchesPlanName('VibeXP - Power User', 'Power User')).toBe(true)
      })

      it('returns true when second name contains first name', () => {
        expect(matchesPlanName('Professional', 'VibeXP - Professional')).toBe(
          true
        )
        expect(matchesPlanName('Starter', 'VibeXP - Starter')).toBe(true)
        expect(matchesPlanName('Power User', 'VibeXP - Power User')).toBe(true)
      })

      it('returns true for substring match regardless of casing', () => {
        expect(matchesPlanName('vibexp - professional', 'PROFESSIONAL')).toBe(
          true
        )
        expect(matchesPlanName('VIBEXP - STARTER', 'starter')).toBe(true)
        expect(matchesPlanName('ViBeXp - PoWeR uSeR', 'power user')).toBe(true)
      })

      it('returns true for partial word matches', () => {
        expect(matchesPlanName('Professional Plan', 'Professional')).toBe(true)
        expect(matchesPlanName('Professional', 'prof')).toBe(true)
        expect(matchesPlanName('Starter Package', 'Start')).toBe(true)
      })
    })

    describe('Non-matches', () => {
      it('returns false for completely different plan names', () => {
        expect(matchesPlanName('Professional', 'Starter')).toBe(false)
        expect(matchesPlanName('Power User', 'Free')).toBe(false)
        expect(matchesPlanName('Enterprise', 'Basic')).toBe(false)
      })

      it('returns false when names have no common substring', () => {
        expect(matchesPlanName('abc', 'def')).toBe(false)
        expect(matchesPlanName('Plan A', 'Plan B')).toBe(false)
      })

      it('returns false for empty strings', () => {
        expect(matchesPlanName('', '')).toBe(true) // Empty strings match each other
        expect(matchesPlanName('Professional', '')).toBe(true) // Any string contains empty string
        expect(matchesPlanName('', 'Professional')).toBe(true) // Empty string is contained in any string
      })
    })

    describe('Edge cases', () => {
      it('handles special characters', () => {
        expect(matchesPlanName('Plan-A', 'Plan-A')).toBe(true)
        expect(matchesPlanName('Plan_B', 'plan_b')).toBe(true)
        expect(matchesPlanName('VibeXP - Pro', 'Pro')).toBe(true)
      })

      it('handles numbers in plan names', () => {
        expect(matchesPlanName('Plan 1', 'plan 1')).toBe(true)
        expect(matchesPlanName('VibeXP - Plan 2', 'Plan 2')).toBe(true)
        expect(matchesPlanName('Pro 2023', '2023')).toBe(true)
      })

      it('handles very long plan names', () => {
        const longName1 =
          'VibeXP Professional Enterprise Premium Plus Edition 2024'
        const longName2 = 'Professional Enterprise Premium'
        expect(matchesPlanName(longName1, longName2)).toBe(true)
      })

      it('handles single character plan names', () => {
        expect(matchesPlanName('A', 'a')).toBe(true)
        expect(matchesPlanName('ABC', 'A')).toBe(true)
        expect(matchesPlanName('B', 'A')).toBe(false)
      })

      it('handles unicode and special characters', () => {
        expect(matchesPlanName('Plañ Professional', 'plañ professional')).toBe(
          true
        )
        expect(matchesPlanName('Plan 日本語', 'plan 日本語')).toBe(true)
      })

      it('is case-insensitive throughout', () => {
        expect(matchesPlanName('MiXeD CaSe PlAn', 'mixed case plan')).toBe(true)
        expect(matchesPlanName('UPPERCASE', 'lowercase')).toBe(false)
      })
    })

    describe('Real-world scenarios', () => {
      it('matches backend plan names with frontend display names', () => {
        // Backend might return just "professional"
        // Frontend displays "VibeXP - Professional"
        expect(matchesPlanName('professional', 'VibeXP - Professional')).toBe(
          true
        )
        expect(matchesPlanName('starter', 'VibeXP - Starter')).toBe(true)
        expect(matchesPlanName('power user', 'VibeXP - Power User')).toBe(true)
      })

      it('matches plan names from Stripe with display names', () => {
        expect(
          matchesPlanName('VibeXP Professional Monthly', 'Professional')
        ).toBe(true)
        expect(matchesPlanName('VibeXP Starter Annual', 'Starter')).toBe(true)
      })

      it('distinguishes between similar but different plans', () => {
        expect(matchesPlanName('Professional', 'Professional Plus')).toBe(true) // Contains
        expect(matchesPlanName('Pro', 'Professional')).toBe(true) // Substring
        expect(matchesPlanName('Starter', 'Advanced Starter')).toBe(true) // Contains
      })
    })
  })

  describe('getCurrentPlanFeatures', () => {
    // Sample paid plans that would come from the API
    const mockPaidPlans: ProductConfiguration[] = [
      {
        id: 'price_starter',
        name: 'VibeXP - Starter',
        price_id: 'price_starter_monthly',
        currency: 'EUR',
        amount: 999,
        popular: false,
        marketing_features: [
          '2 AI Tool Integration',
          '500 AI Tool Session',
          '200 Prompts',
          '200 Artifacts',
          '200 Memory',
          '3 AI Agent Integration',
          '300 Agentic Conversation',
          'MCP included',
        ],
      },
      {
        id: 'price_professional',
        name: 'VibeXP - Professional',
        price_id: 'price_professional_monthly',
        currency: 'EUR',
        amount: 1999,
        popular: true,
        marketing_features: [
          '3 AI Tool Integration',
          '1000 AI Tool Session',
          '500 Prompts',
          '500 Artifacts',
          '500 Memory',
          '5 AI Agent Integration',
          '600 Agentic Conversation',
          'MCP included',
        ],
      },
      {
        id: 'price_power_user',
        name: 'VibeXP - Power User',
        price_id: 'price_power_user_monthly',
        currency: 'EUR',
        amount: 2999,
        popular: false,
        marketing_features: [
          'Unlimited AI Tools integration',
          '2000 AI Tool session',
          '1000 Prompts',
          '1000 Artifacts',
          '1000 Memory',
          'Unlimited AI Agent Integration',
          '1500 Agentic Conversation',
          'MCP included',
        ],
      },
    ]

    it('returns free plan features for free status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.BASIC,
        plan_name: 'Free Plan',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription)

      expect(features).toEqual([
        '100 API calls/month',
        '50 MB storage',
        'Basic support',
        'Core features',
        'Community access',
      ])
    })

    it('returns free plan features for none status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.NONE,
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: false,
      }

      const features = getCurrentPlanFeatures(subscription)

      expect(features).toEqual([
        '100 API calls/month',
        '50 MB storage',
        'Basic support',
        'Core features',
        'Community access',
      ])
    })

    it('returns free plan features when plan_name is null', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: null,
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription)

      expect(features).toEqual([
        '100 API calls/month',
        '50 MB storage',
        'Basic support',
        'Core features',
        'Community access',
      ])
    })

    it('returns matching plan features when plan name matches exactly', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: 'VibeXP - Starter',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription, mockPaidPlans)

      expect(features).toEqual([
        '2 AI Tool Integration',
        '500 AI Tool Session',
        '200 Prompts',
        '200 Artifacts',
        '200 Memory',
        '3 AI Agent Integration',
        '300 Agentic Conversation',
        'MCP included',
      ])
    })

    it('returns matching plan features using substring matching (backend name vs display name)', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: 'professional', // Backend returns just "professional"
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription, mockPaidPlans)

      expect(features).toContain('3 AI Tool Integration')
      expect(features).toContain('1000 AI Tool Session')
      expect(features).toContain('500 Prompts')
      expect(features).toContain('MCP included')
    })

    it('returns matching plan features when display name is substring of backend name', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: 'VibeXP - Professional Plan Extra Long', // Backend returns longer name
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription, mockPaidPlans)

      expect(features).toContain('3 AI Tool Integration')
      expect(features).toContain('1000 AI Tool Session')
    })

    it('returns matching plan features case-insensitively', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: 'POWER USER', // Different casing
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription, mockPaidPlans)

      expect(features).toContain('Unlimited AI Tools integration')
      expect(features).toContain('MCP included')
    })

    it('returns free plan features when paidPlans is empty', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: 'Professional Plan',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription, [])

      expect(features).toEqual([
        '100 API calls/month',
        '50 MB storage',
        'Basic support',
        'Core features',
        'Community access',
      ])
    })

    it('returns free plan features when paidPlans is undefined', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: 'Professional Plan',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription, undefined)

      expect(features).toEqual([
        '100 API calls/month',
        '50 MB storage',
        'Basic support',
        'Core features',
        'Community access',
      ])
    })

    it('returns free plan features for unknown plan names', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: 'Unknown Super Plan',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      const features = getCurrentPlanFeatures(subscription, mockPaidPlans)

      expect(features).toEqual([
        '100 API calls/month',
        '50 MB storage',
        'Basic support',
        'Core features',
        'Community access',
      ])
    })
  })

  describe('shouldShowUpgradeOptions', () => {
    it('returns true for free status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.BASIC,
        plan_name: 'Free Plan',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      expect(shouldShowUpgradeOptions(subscription)).toBe(true)
    })

    it('returns true for trial_active status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.TRIAL_ACTIVE,
        plan_name: 'Trial Plan',
        trial_end: '2024-02-15T00:00:00Z',
        is_trial_active: true,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      expect(shouldShowUpgradeOptions(subscription)).toBe(true)
    })

    it('returns true for cancelled status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.CANCELLED,
        plan_name: 'Cancelled Plan',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: false,
      }

      expect(shouldShowUpgradeOptions(subscription)).toBe(true)
    })

    it('returns true for past_due status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.PAST_DUE,
        plan_name: 'Pro Plan',
        current_period_end: '2024-01-15T00:00:00Z',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: false,
      }

      expect(shouldShowUpgradeOptions(subscription)).toBe(true)
    })

    it('returns true for unpaid status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.UNPAID,
        plan_name: 'Pro Plan',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: false,
      }

      expect(shouldShowUpgradeOptions(subscription)).toBe(true)
    })

    it('returns true for none status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.NONE,
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: false,
      }

      expect(shouldShowUpgradeOptions(subscription)).toBe(true)
    })

    it('returns false for active status', () => {
      const subscription: SubscriptionStatusResponse = {
        status: SUBSCRIPTION_STATUS.ACTIVE,
        plan_name: 'Pro Plan',
        current_period_start: '2024-01-01T00:00:00Z',
        current_period_end: '2024-02-01T00:00:00Z',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
      }

      expect(shouldShowUpgradeOptions(subscription)).toBe(false)
    })
  })

  describe('getPaidPlans', () => {
    const allPlans: ProductConfiguration[] = [
      {
        id: 'free',
        name: 'Free',
        price_id: 'price_free',
        currency: 'EUR',
        amount: 0,
        popular: false,
        marketing_features: ['Feature 1'],
      },
      {
        id: 'starter',
        name: 'Starter',
        price_id: 'price_starter',
        currency: 'EUR',
        amount: 999,
        popular: false,
        marketing_features: ['Feature 1', 'Feature 2'],
      },
      {
        id: 'pro',
        name: 'Professional',
        price_id: 'price_pro',
        currency: 'EUR',
        amount: 1999,
        popular: true,
        marketing_features: ['Feature 1', 'Feature 2', 'Feature 3'],
      },
      {
        id: 'power_user',
        name: 'Power User',
        price_id: 'price_power_user',
        currency: 'EUR',
        amount: 2999,
        popular: false,
        marketing_features: ['All features'],
      },
    ]

    it('filters out free plans (amount = 0)', () => {
      const result = getPaidPlans(allPlans)

      expect(result).not.toContainEqual(expect.objectContaining({ amount: 0 }))
      expect(result).not.toContainEqual(expect.objectContaining({ id: 'free' }))
    })

    it('sorts plans by amount in ascending order', () => {
      const result = getPaidPlans(allPlans)

      // Should have 3 plans after filtering (free plan is removed)
      expect(result).toHaveLength(3)
      expect(result[0].amount).toBeLessThan(result[1].amount)
      expect(result[1].amount).toBeLessThan(result[2].amount)
    })

    it('returns plans in correct order', () => {
      const result = getPaidPlans(allPlans)

      expect(result).toEqual([
        expect.objectContaining({ id: 'starter', amount: 999 }),
        expect.objectContaining({ id: 'pro', amount: 1999 }),
        expect.objectContaining({ id: 'power_user', amount: 2999 }),
      ])
    })

    it('handles empty input array', () => {
      const result = getPaidPlans([])

      expect(result).toEqual([])
    })

    it('handles array with only free plans', () => {
      const onlyFreePlans: ProductConfiguration[] = [
        {
          id: 'free',
          name: 'Free',
          price_id: 'price_free',
          currency: 'EUR',
          amount: 0,
          popular: false,
          marketing_features: ['Feature 1'],
        },
      ]

      const result = getPaidPlans(onlyFreePlans)

      expect(result).toEqual([])
    })

    it('preserves all plan properties', () => {
      const result = getPaidPlans(allPlans)

      result.forEach(plan => {
        expect(plan).toHaveProperty('id')
        expect(plan).toHaveProperty('name')
        expect(plan).toHaveProperty('price_id')
        expect(plan).toHaveProperty('currency')
        expect(plan).toHaveProperty('amount')
        expect(plan).toHaveProperty('popular')
        expect(plan).toHaveProperty('marketing_features')
      })
    })
  })
})
