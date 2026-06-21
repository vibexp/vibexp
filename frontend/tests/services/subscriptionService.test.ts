import type {
  CreateSubscriptionRequest,
  CreateSubscriptionResponse,
  SubscriptionStatusResponse,
  CreatePortalSessionResponse,
  ProductConfigurationResponse,
  ResourceUsageResponse,
} from '../../src/types'

// Mock apiClient
const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
}

jest.mock('../../src/lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Import after mocking
import { subscriptionService } from '../../src/services/subscriptionService'

describe('SubscriptionService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('createSubscription', () => {
    it('should create subscription with correct payload', async () => {
      const request: CreateSubscriptionRequest = {
        price_id: 'price_123',
        ga4_client_id: 'client_456',
        allow_promotion_codes: true,
      }

      const mockResponse: CreateSubscriptionResponse = {
        checkout_url: 'https://checkout.stripe.com/session_123',
        session_id: 'cs_test_123',
      }

      mockApiClient.post.mockResolvedValue(mockResponse)

      const result = await subscriptionService.createSubscription(request)

      expect(mockApiClient.post).toHaveBeenCalledWith(
        '/subscriptions/create',
        request
      )
      expect(result).toEqual(mockResponse)
    })

    it('should handle API errors', async () => {
      mockApiClient.post.mockRejectedValue(new Error('API Error'))

      await expect(
        subscriptionService.createSubscription({
          price_id: 'price_123',
        })
      ).rejects.toThrow('API Error')
    })
  })

  describe('getSubscriptionStatus', () => {
    it('should fetch subscription status', async () => {
      const mockResponse: SubscriptionStatusResponse = {
        status: 'active',
        plan_name: 'Pro',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: true,
        current_period_end: '2025-01-01T00:00:00Z',
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await subscriptionService.getSubscriptionStatus()

      expect(mockApiClient.get).toHaveBeenCalledWith('/subscriptions/status')
      expect(result).toEqual(mockResponse)
    })

    it('should handle trial subscription status', async () => {
      const mockResponse: SubscriptionStatusResponse = {
        status: 'trial_active',
        plan_name: 'Pro',
        is_trial_active: true,
        cancel_at_period_end: false,
        can_access_service: true,
        trial_end: '2025-01-15T00:00:00Z',
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await subscriptionService.getSubscriptionStatus()

      expect(result.is_trial_active).toBe(true)
      expect(result.status).toBe('trial_active')
    })

    it('should handle inactive subscription', async () => {
      const mockResponse: SubscriptionStatusResponse = {
        status: 'none',
        is_trial_active: false,
        cancel_at_period_end: false,
        can_access_service: false,
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await subscriptionService.getSubscriptionStatus()

      expect(result.can_access_service).toBe(false)
    })
  })

  describe('createPortalSession', () => {
    it('should create portal session', async () => {
      const mockResponse: CreatePortalSessionResponse = {
        url: 'https://billing.stripe.com/portal_123',
      }

      mockApiClient.post.mockResolvedValue(mockResponse)

      const result = await subscriptionService.createPortalSession()

      expect(mockApiClient.post).toHaveBeenCalledWith(
        '/subscriptions/portal-session'
      )
      expect(result).toEqual(mockResponse)
    })
  })

  describe('getProductConfiguration', () => {
    it('should fetch product configuration', async () => {
      const mockResponse: ProductConfigurationResponse = {
        products: [
          {
            id: 'prod_123',
            name: 'Pro Plan',
            price_id: 'price_123',
            currency: 'usd',
            amount: 1999,
            popular: true,
            marketing_features: ['Unlimited prompts', 'Priority support'],
          },
        ],
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await subscriptionService.getProductConfiguration()

      expect(mockApiClient.get).toHaveBeenCalledWith('/subscriptions/products')
      expect(result).toEqual(mockResponse)
    })

    it('should handle multiple products', async () => {
      const mockResponse: ProductConfigurationResponse = {
        products: [
          {
            id: 'prod_basic',
            name: 'Basic Plan',
            price_id: 'price_basic',
            currency: 'usd',
            amount: 999,
            popular: false,
            marketing_features: ['100 prompts'],
          },
          {
            id: 'prod_pro',
            name: 'Pro Plan',
            price_id: 'price_pro',
            currency: 'usd',
            amount: 1999,
            popular: true,
            marketing_features: ['Unlimited prompts'],
          },
        ],
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await subscriptionService.getProductConfiguration()

      expect(result.products).toHaveLength(2)
    })
  })

  describe('getResourceUsage', () => {
    it('should fetch resource usage', async () => {
      const mockResponse: ResourceUsageResponse = {
        user_id: 'user-123',
        resources: [
          {
            resource_type: 'prompts',
            count: 10,
            limit: 100,
            individual_limit: 100,
            team_quota: 0,
            percentage: 10,
          },
          {
            resource_type: 'artifacts',
            count: 5,
            limit: 50,
            individual_limit: 50,
            team_quota: 0,
            percentage: 10,
          },
        ],
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await subscriptionService.getResourceUsage()

      expect(mockApiClient.get).toHaveBeenCalledWith('/resource-usage')
      expect(result).toEqual(mockResponse)
    })

    it('should handle empty resources', async () => {
      const mockResponse: ResourceUsageResponse = {
        user_id: 'user-123',
        resources: [],
      }

      mockApiClient.get.mockResolvedValue(mockResponse)

      const result = await subscriptionService.getResourceUsage()

      expect(result.resources).toHaveLength(0)
    })
  })
})
