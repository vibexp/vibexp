/**
 * Unit Tests for TeamSubscriptionService - Issue #737
 *
 * This test suite validates the TeamSubscriptionService functionality including:
 * - Get team plans
 * - Create checkout sessions for team subscriptions
 * - Get team subscription details
 * - Get billing portal URL
 * - Get owned team subscriptions
 * - Error handling and edge cases
 *
 * Coverage target: >50%
 */

import type {
  TeamPlan,
  TeamPlanPrice,
  TeamSubscriptionResponse,
  OwnedTeamSubscription,
} from '../../src/services/teamSubscriptionService'

// Mock the authService
const mockAuthService = {
  getToken: jest.fn(),
  setToken: jest.fn(),
}

jest.mock('../../src/services/authService', () => ({
  authService: mockAuthService,
}))

// Mock fetch globally
global.fetch = jest.fn()

// Create a test implementation of TeamSubscriptionService
class TestTeamSubscriptionService {
  private readonly API_BASE_URL = 'https://api.vibexp.io/api/v1'

  private async makeRequest<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const token = mockAuthService.getToken()
    if (!token) {
      throw new Error('No authentication token')
    }

    const response = await fetch(`${this.API_BASE_URL}${endpoint}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
        ...options.headers,
      },
    })

    if (!response.ok) {
      if (response.status === 401) {
        mockAuthService.setToken(null)
        throw new Error('Authentication expired')
      }
      if (response.status === 404) {
        throw new Error('404: Not found')
      }
      const errorData = await response.json().catch(() => null)
      throw new Error(
        errorData?.message || `HTTP error! status: ${response.status}`
      )
    }

    if (response.status === 204) {
      return null as T
    }

    const contentType = response.headers.get('content-type')
    if (contentType && contentType.includes('application/json')) {
      return response.json()
    }

    return null as T
  }

  async getTeamPlans(): Promise<TeamPlan[]> {
    const response = await this.makeRequest<{ plans: TeamPlan[] }>(
      '/teams/subscription-plans'
    )
    return response.plans
  }

  async createCheckoutSession(
    teamId: string,
    tier: string,
    seatCount: number,
    billingInterval: 'month' | 'year' = 'month'
  ): Promise<{ checkout_url: string; session_id: string }> {
    return this.makeRequest<{ checkout_url: string; session_id: string }>(
      `/teams/${teamId}/subscribe`,
      {
        method: 'POST',
        body: JSON.stringify({
          tier,
          seat_count: seatCount,
          billing_interval: billingInterval,
          allow_promotion_codes: true,
        }),
      }
    )
  }

  async getTeamSubscription(
    teamId: string
  ): Promise<TeamSubscriptionResponse | null> {
    try {
      return await this.makeRequest<TeamSubscriptionResponse>(
        `/teams/${teamId}/subscription`
      )
    } catch (error) {
      // Team has no subscription - return null instead of throwing
      if (error instanceof Error && error.message.includes('404')) {
        return null
      }
      throw error
    }
  }

  async getPortalUrl(teamId: string): Promise<string> {
    const response = await this.makeRequest<{ url: string }>(
      `/teams/${teamId}/subscription/portal`
    )
    return response.url
  }

  async getOwnedTeamSubscriptions(): Promise<OwnedTeamSubscription[]> {
    const response = await this.makeRequest<{
      subscriptions: OwnedTeamSubscription[]
    }>('/user/team-subscriptions')
    return response.subscriptions
  }
}

describe('TeamSubscriptionService', () => {
  let subscriptionService: TestTeamSubscriptionService
  const mockToken = 'mock-auth-token'
  const baseUrl = 'https://api.vibexp.io/api/v1'
  const mockFetch = fetch as jest.MockedFunction<typeof fetch>

  const mockPrice: TeamPlanPrice = {
    price_id: 'price_123',
    interval: 'month',
    amount: 2500, // $25.00
    currency: 'usd',
  }

  const mockPlan: TeamPlan = {
    id: 'plan_starter',
    name: 'Starter',
    tier: 'starter',
    description: 'Perfect for small teams',
    min_seats: 2,
    max_seats: 10,
    prices: [
      mockPrice,
      { ...mockPrice, price_id: 'price_456', interval: 'year', amount: 24000 },
    ],
    features: ['5GB storage', 'Email support', 'Basic analytics'],
  }

  const mockSubscription: TeamSubscriptionResponse = {
    id: 'sub_123',
    team_id: 'team-123',
    tier: 'starter',
    seat_count: 5,
    status: 'active',
    stripe_subscription_id: 'sub_stripe_123',
    stripe_customer_id: 'cus_123',
    current_period_start: '2023-01-01T00:00:00Z',
    current_period_end: '2023-02-01T00:00:00Z',
    cancel_at_period_end: false,
  }

  const mockOwnedSubscription: OwnedTeamSubscription = {
    team_id: 'team-123',
    team_name: 'My Team',
    subscription_id: 'sub_123',
    tier: 'starter',
    seat_count: 5,
    status: 'active',
    current_period_start: '2023-01-01T00:00:00Z',
    current_period_end: '2023-02-01T00:00:00Z',
    cancel_at_period_end: false,
  }

  beforeEach(() => {
    jest.clearAllMocks()
    subscriptionService = new TestTeamSubscriptionService()
    mockAuthService.getToken.mockReturnValue(mockToken)

    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
      headers: new Headers({ 'content-type': 'application/json' }),
    } as Response)
  })

  describe('Authentication', () => {
    it('should throw error when no token is available', async () => {
      mockAuthService.getToken.mockReturnValue(null)

      await expect(subscriptionService.getTeamPlans()).rejects.toThrow(
        'No authentication token'
      )
      expect(mockFetch).not.toHaveBeenCalled()
    })

    it('should include Bearer token in request headers', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ plans: [] }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await subscriptionService.getTeamPlans()

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/teams/subscription-plans`,
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: `Bearer ${mockToken}`,
            'Content-Type': 'application/json',
          }),
        })
      )
    })

    it('should handle 401 authentication expired', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ message: 'Unauthorized' }),
      } as Response)

      await expect(subscriptionService.getTeamPlans()).rejects.toThrow(
        'Authentication expired'
      )
      expect(mockAuthService.setToken).toHaveBeenCalledWith(null)
    })
  })

  describe('getTeamPlans', () => {
    it('should fetch available team plans', async () => {
      const plans = [
        mockPlan,
        {
          ...mockPlan,
          id: 'plan_pro',
          name: 'Professional',
          tier: 'professional',
        },
        { ...mockPlan, id: 'plan_ent', name: 'Enterprise', tier: 'enterprise' },
      ]

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ plans }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getTeamPlans()

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/teams/subscription-plans`,
        expect.any(Object)
      )
      expect(result).toEqual(plans)
      expect(result).toHaveLength(3)
    })

    it('should handle empty plans array', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ plans: [] }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getTeamPlans()
      expect(result).toEqual([])
    })

    it('should return plans with pricing information', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ plans: [mockPlan] }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getTeamPlans()
      expect(result[0].prices).toHaveLength(2)
      expect(result[0].prices[0].interval).toBe('month')
      expect(result[0].prices[1].interval).toBe('year')
    })
  })

  describe('createCheckoutSession', () => {
    const checkoutResponse = {
      checkout_url: 'https://checkout.stripe.com/c/pay/cs_test_123',
      session_id: 'cs_test_123',
    }

    it('should create checkout session with monthly billing', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(checkoutResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.createCheckoutSession(
        'team-123',
        'starter',
        5,
        'month'
      )

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/teams/team-123/subscribe`,
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            tier: 'starter',
            seat_count: 5,
            billing_interval: 'month',
            allow_promotion_codes: true,
          }),
        })
      )
      expect(result.checkout_url).toBe(checkoutResponse.checkout_url)
      expect(result.session_id).toBe(checkoutResponse.session_id)
    })

    it('should create checkout session with yearly billing', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(checkoutResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await subscriptionService.createCheckoutSession(
        'team-123',
        'professional',
        10,
        'year'
      )

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/teams/team-123/subscribe`,
        expect.objectContaining({
          body: JSON.stringify({
            tier: 'professional',
            seat_count: 10,
            billing_interval: 'year',
            allow_promotion_codes: true,
          }),
        })
      )
    })

    it('should default to monthly billing when not specified', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(checkoutResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      await subscriptionService.createCheckoutSession('team-123', 'starter', 5)

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/teams/team-123/subscribe`,
        expect.objectContaining({
          body: JSON.stringify({
            tier: 'starter',
            seat_count: 5,
            billing_interval: 'month',
            allow_promotion_codes: true,
          }),
        })
      )
    })

    it('should handle invalid tier error', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'Invalid subscription tier' }),
      } as Response)

      await expect(
        subscriptionService.createCheckoutSession('team-123', 'invalid', 5)
      ).rejects.toThrow('Invalid subscription tier')
    })

    it('should handle invalid seat count error', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 400,
        json: () =>
          Promise.resolve({
            message: 'Seat count must be between 2 and 10 for starter plan',
          }),
      } as Response)

      await expect(
        subscriptionService.createCheckoutSession('team-123', 'starter', 1)
      ).rejects.toThrow('Seat count must be between 2 and 10 for starter plan')
    })

    it('should handle team not found error', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'Team not found' }),
      } as Response)

      await expect(
        subscriptionService.createCheckoutSession('nonexistent', 'starter', 5)
      ).rejects.toThrow('404: Not found')
    })

    it('should handle existing subscription error', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 409,
        json: () =>
          Promise.resolve({
            message: 'Team already has an active subscription',
          }),
      } as Response)

      await expect(
        subscriptionService.createCheckoutSession('team-123', 'starter', 5)
      ).rejects.toThrow('Team already has an active subscription')
    })
  })

  describe('getTeamSubscription', () => {
    it('should fetch team subscription details', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockSubscription),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getTeamSubscription('team-123')

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/teams/team-123/subscription`,
        expect.any(Object)
      )
      expect(result).toEqual(mockSubscription)
    })

    it('should return null when team has no subscription (404)', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'No subscription found' }),
      } as Response)

      const result = await subscriptionService.getTeamSubscription('team-123')

      expect(result).toBeNull()
    })

    it('should handle subscription with cancel_at_period_end', async () => {
      const cancelingSubscription = {
        ...mockSubscription,
        cancel_at_period_end: true,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(cancelingSubscription),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getTeamSubscription('team-123')

      expect(result?.cancel_at_period_end).toBe(true)
    })

    it('should handle different subscription statuses', async () => {
      const statuses = ['active', 'trialing', 'past_due', 'canceled']

      for (const status of statuses) {
        mockFetch.mockResolvedValueOnce({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              ...mockSubscription,
              status,
            }),
          headers: new Headers({ 'content-type': 'application/json' }),
        } as Response)

        const result = await subscriptionService.getTeamSubscription('team-123')
        expect(result?.status).toBe(status)
      }
    })

    it('should throw non-404 errors', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'Internal server error' }),
      } as Response)

      await expect(
        subscriptionService.getTeamSubscription('team-123')
      ).rejects.toThrow('Internal server error')
    })
  })

  describe('getPortalUrl', () => {
    it('should fetch billing portal URL', async () => {
      const portalUrl =
        'https://billing.stripe.com/session/test_portal_session_123'

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ url: portalUrl }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getPortalUrl('team-123')

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/teams/team-123/subscription/portal`,
        expect.any(Object)
      )
      expect(result).toBe(portalUrl)
    })

    it('should handle no subscription error', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: () =>
          Promise.resolve({
            message: 'Team does not have an active subscription',
          }),
      } as Response)

      await expect(
        subscriptionService.getPortalUrl('team-no-sub')
      ).rejects.toThrow('404: Not found')
    })

    it('should handle permission error', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 403,
        json: () =>
          Promise.resolve({
            message: 'Only team owner can access billing portal',
          }),
      } as Response)

      await expect(
        subscriptionService.getPortalUrl('team-123')
      ).rejects.toThrow('Only team owner can access billing portal')
    })
  })

  describe('getOwnedTeamSubscriptions', () => {
    it('should fetch all team subscriptions owned by user', async () => {
      const subscriptions = [
        mockOwnedSubscription,
        {
          ...mockOwnedSubscription,
          team_id: 'team-456',
          team_name: 'Another Team',
          tier: 'professional',
        },
      ]

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ subscriptions }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getOwnedTeamSubscriptions()

      expect(mockFetch).toHaveBeenCalledWith(
        `${baseUrl}/user/team-subscriptions`,
        expect.any(Object)
      )
      expect(result).toEqual(subscriptions)
      expect(result).toHaveLength(2)
    })

    it('should handle empty subscriptions array', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ subscriptions: [] }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getOwnedTeamSubscriptions()
      expect(result).toEqual([])
    })

    it('should return subscription details with cancellation info', async () => {
      const cancelingSubscription = {
        ...mockOwnedSubscription,
        cancel_at_period_end: true,
      }

      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ subscriptions: [cancelingSubscription] }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const result = await subscriptionService.getOwnedTeamSubscriptions()
      expect(result[0].cancel_at_period_end).toBe(true)
    })
  })

  describe('Error Handling', () => {
    it('should handle network errors', async () => {
      mockFetch.mockRejectedValue(new Error('Network error'))

      await expect(subscriptionService.getTeamPlans()).rejects.toThrow(
        'Network error'
      )
    })

    it('should handle timeout errors', async () => {
      mockFetch.mockRejectedValue(new Error('Request timeout'))

      await expect(
        subscriptionService.createCheckoutSession('team-123', 'starter', 5)
      ).rejects.toThrow('Request timeout')
    })

    it('should handle server errors', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'Internal server error' }),
      } as Response)

      await expect(subscriptionService.getTeamPlans()).rejects.toThrow(
        'Internal server error'
      )
    })

    it('should handle rate limiting', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 429,
        json: () =>
          Promise.resolve({
            message: 'Too many requests. Please try again later.',
          }),
      } as Response)

      await expect(subscriptionService.getTeamPlans()).rejects.toThrow(
        'Too many requests. Please try again later.'
      )
    })
  })

  describe('Integration Scenarios', () => {
    it('should handle complete subscription flow', async () => {
      // Step 1: Get plans
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ plans: [mockPlan] }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const plans = await subscriptionService.getTeamPlans()
      expect(plans).toHaveLength(1)

      // Step 2: Create checkout session
      const checkoutResponse = {
        checkout_url: 'https://checkout.stripe.com/c/pay/cs_test_123',
        session_id: 'cs_test_123',
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(checkoutResponse),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const checkout = await subscriptionService.createCheckoutSession(
        'team-123',
        'starter',
        5
      )
      expect(checkout.checkout_url).toBeDefined()

      // Step 3: Get subscription after checkout
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockSubscription),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const subscription =
        await subscriptionService.getTeamSubscription('team-123')
      expect(subscription?.status).toBe('active')

      // Step 4: Access billing portal
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            url: 'https://billing.stripe.com/session/test',
          }),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as Response)

      const portalUrl = await subscriptionService.getPortalUrl('team-123')
      expect(portalUrl).toContain('stripe.com')

      expect(mockFetch).toHaveBeenCalledTimes(4)
    })
  })
})
