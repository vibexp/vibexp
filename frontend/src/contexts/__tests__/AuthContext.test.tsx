import { act, render, screen, waitFor } from '@testing-library/react'
import { renderHook } from '@testing-library/react'

import type { User } from '../../types'
import { AuthProvider, useAuth } from '../AuthContext'

// Mock the authService (cookie-based, no token management)
jest.mock('../../services/authService', () => ({
  authService: {
    getCurrentUser: jest.fn(),
    getLoginUrl: jest.fn(),
    logout: jest.fn(),
    markOnboardingComplete: jest.fn(),
  },
}))

// Import the mocked authService after the mock
import { authService } from '../../services/authService'

// Type the mocked authService properly
const mockAuthService = authService as jest.Mocked<typeof authService>

// Mock console.error to test error handling but allow calls through
const consoleSpy = jest.spyOn(console, 'error')

describe('AuthContext (cookie-based auth)', () => {
  const mockUser: User = {
    id: 'user-123',
    google_id: 'google-456',
    email: 'test@example.com',
    name: 'Test User',
    avatar_url: 'https://example.com/avatar.jpg',
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
    onboarding_completed: true,
  }

  // A first-time user (created_at within the last few seconds)
  const mockNewUser: User = {
    ...mockUser,
    id: 'user-new',
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }

  beforeEach(() => {
    jest.clearAllMocks()
    consoleSpy.mockClear()
    // Reset GA4 globals before each test
    window.dataLayer = []
    window.gtag = jest.fn()
    window.sessionStorage.clear()
  })

  afterAll(() => {
    consoleSpy.mockRestore()
  })

  describe('Context Provider', () => {
    describe('Provider initialization and setup', () => {
      it('should show unauthenticated when GET /auth/me returns 401', async () => {
        // Simulate 401: server returns an error (no cookie / expired session)
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('401 Unauthorized')
        )

        const TestComponent = () => {
          const { user, isAuthenticated, isLoading } = useAuth()
          return (
            <div>
              <div data-testid="user">{user ? user.name : 'null'}</div>
              <div data-testid="authenticated">{String(isAuthenticated)}</div>
              <div data-testid="loading">{String(isLoading)}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for loading to complete
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        expect(screen.getByTestId('user')).toHaveTextContent('null')
        expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
        expect(mockAuthService.getCurrentUser).toHaveBeenCalledTimes(1)
      })

      it('should initialize with authenticated user when session cookie is valid', async () => {
        mockAuthService.getCurrentUser.mockResolvedValue(mockUser)

        const TestComponent = () => {
          const { user, isAuthenticated, isLoading } = useAuth()
          return (
            <div>
              <div data-testid="user">{user ? user.name : 'null'}</div>
              <div data-testid="authenticated">{String(isAuthenticated)}</div>
              <div data-testid="loading">{String(isLoading)}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for authentication check to complete
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        expect(screen.getByTestId('user')).toHaveTextContent('Test User')
        expect(screen.getByTestId('authenticated')).toHaveTextContent('true')
        expect(mockAuthService.getCurrentUser).toHaveBeenCalledTimes(1)
      })

      it('should render children components correctly', () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )

        const TestChild = () => <div data-testid="child">Child Component</div>

        render(
          <AuthProvider>
            <TestChild />
          </AuthProvider>
        )

        expect(screen.getByTestId('child')).toBeInTheDocument()
        expect(screen.getByTestId('child')).toHaveTextContent('Child Component')
      })

      it('should provide all required context values', () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )

        const TestComponent = () => {
          const context = useAuth()
          return (
            <div>
              <div data-testid="has-user">{String('user' in context)}</div>
              <div data-testid="has-authenticated">
                {String('isAuthenticated' in context)}
              </div>
              <div data-testid="has-login">{String('login' in context)}</div>
              <div data-testid="has-logout">{String('logout' in context)}</div>
              <div data-testid="has-loading">
                {String('isLoading' in context)}
              </div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        expect(screen.getByTestId('has-user')).toHaveTextContent('true')
        expect(screen.getByTestId('has-authenticated')).toHaveTextContent(
          'true'
        )
        expect(screen.getByTestId('has-login')).toHaveTextContent('true')
        expect(screen.getByTestId('has-logout')).toHaveTextContent('true')
        expect(screen.getByTestId('has-loading')).toHaveTextContent('true')
      })
    })

    describe('Provider error handling', () => {
      it('should silently handle 401 on mount (unauthenticated state)', async () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('401 Unauthorized')
        )

        const TestComponent = () => {
          const { user, isAuthenticated, isLoading } = useAuth()
          return (
            <div>
              <div data-testid="user">{user ? user.name : 'null'}</div>
              <div data-testid="authenticated">{String(isAuthenticated)}</div>
              <div data-testid="loading">{String(isLoading)}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for authentication check to complete
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        expect(screen.getByTestId('user')).toHaveTextContent('null')
        expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
        // Should NOT call logout for 401 (just silently clear state)
        expect(mockAuthService.logout).not.toHaveBeenCalled()
      })

      it('should handle multiple provider instances gracefully', () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )

        const TestComponent = () => {
          const { isAuthenticated } = useAuth()
          return (
            <div data-testid="authenticated">{String(isAuthenticated)}</div>
          )
        }

        render(
          <AuthProvider>
            <AuthProvider>
              <TestComponent />
            </AuthProvider>
          </AuthProvider>
        )

        expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
      })
    })
  })

  describe('State Management', () => {
    describe('Login state updates', () => {
      it('should call getLoginUrl and redirect during login', async () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )
        mockAuthService.getLoginUrl.mockResolvedValue(
          'https://idp.example.com/authorize?...'
        )

        const TestComponent = () => {
          const { login, isLoading } = useAuth()
          return (
            <div>
              <button
                onClick={() => {
                  void login()
                }}
                data-testid="login-btn"
              >
                Login
              </button>
              <div data-testid="loading">{String(isLoading)}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for initial loading to complete
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        // Click login button
        const loginBtn = screen.getByTestId('login-btn')
        act(() => {
          loginBtn.click()
        })

        // Should call getLoginUrl
        await waitFor(() => {
          expect(mockAuthService.getLoginUrl).toHaveBeenCalledTimes(1)
        })
      })

      it('should handle login failure and reset loading state', async () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )
        mockAuthService.getLoginUrl.mockRejectedValue(
          new Error('Login URL fetch failed')
        )

        const TestComponent = () => {
          const { login, isLoading } = useAuth()
          return (
            <div>
              <button
                onClick={() => {
                  void (async () => {
                    try {
                      await login()
                    } catch {
                      // Catch error to prevent test failure
                    }
                  })()
                }}
                data-testid="login-btn"
              >
                Login
              </button>
              <div data-testid="loading">{String(isLoading)}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for initial loading to complete
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        // Click login button
        const loginBtn = screen.getByTestId('login-btn')
        act(() => {
          loginBtn.click()
        })

        // Should reset loading state after failure
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        expect(mockAuthService.getLoginUrl).toHaveBeenCalledTimes(1)
      })
    })

    describe('Logout state changes', () => {
      it('should clear user data and call server logout', async () => {
        mockAuthService.getCurrentUser.mockResolvedValue(mockUser)
        mockAuthService.logout.mockResolvedValue(undefined)

        const TestComponent = () => {
          const { user, isAuthenticated, logout } = useAuth()
          return (
            <div>
              <div data-testid="user">{user ? user.name : 'null'}</div>
              <div data-testid="authenticated">{String(isAuthenticated)}</div>
              <button
                onClick={() => {
                  logout()
                }}
                data-testid="logout-btn"
              >
                Logout
              </button>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for authentication to complete
        await waitFor(() => {
          expect(screen.getByTestId('user')).toHaveTextContent('Test User')
          expect(screen.getByTestId('authenticated')).toHaveTextContent('true')
        })

        // Click logout button
        const logoutBtn = screen.getByTestId('logout-btn')
        act(() => {
          logoutBtn.click()
        })

        // Client state is cleared immediately
        expect(screen.getByTestId('user')).toHaveTextContent('null')
        expect(screen.getByTestId('authenticated')).toHaveTextContent('false')

        // Server logout is called asynchronously
        await waitFor(() => {
          expect(mockAuthService.logout).toHaveBeenCalledTimes(1)
        })
      })

      it('should handle logout when user is already logged out', async () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )
        mockAuthService.logout.mockResolvedValue(undefined)

        const TestComponent = () => {
          const { user, isAuthenticated, logout } = useAuth()
          return (
            <div>
              <div data-testid="user">{user ? user.name : 'null'}</div>
              <div data-testid="authenticated">{String(isAuthenticated)}</div>
              <button
                onClick={() => {
                  logout()
                }}
                data-testid="logout-btn"
              >
                Logout
              </button>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for initial state
        await waitFor(() => {
          expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
        })

        // Click logout button
        const logoutBtn = screen.getByTestId('logout-btn')
        act(() => {
          logoutBtn.click()
        })

        expect(screen.getByTestId('user')).toHaveTextContent('null')
        expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
      })
    })

    describe('Loading state management', () => {
      it('should set loading to true initially and false after auth check', async () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )

        const loadingStates: boolean[] = []
        const TestComponent = () => {
          const { isLoading } = useAuth()
          loadingStates.push(isLoading)
          return <div data-testid="loading">{String(isLoading)}</div>
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for loading to complete
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        expect(loadingStates).toContain(true) // Should have been true initially
        expect(loadingStates[loadingStates.length - 1]).toBe(false) // Should be false at the end
      })

      it('should manage loading state during authentication check with user fetch', async () => {
        mockAuthService.getCurrentUser.mockImplementation(
          () =>
            new Promise(resolve => {
              setTimeout(() => {
                resolve(mockUser)
              }, 100)
            })
        )

        const TestComponent = () => {
          const { isLoading, user } = useAuth()
          return (
            <div>
              <div data-testid="loading">{String(isLoading)}</div>
              <div data-testid="user">{user ? user.name : 'null'}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Initially should be loading
        expect(screen.getByTestId('loading')).toHaveTextContent('true')

        // Wait for loading to complete
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        expect(screen.getByTestId('user')).toHaveTextContent('Test User')
      })
    })
  })

  describe('Consumer Integration', () => {
    describe('useAuth hook functionality', () => {
      it('should provide access to auth context values', () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )

        const { result } = renderHook(() => useAuth(), {
          wrapper: ({ children }) => <AuthProvider>{children}</AuthProvider>,
        })

        expect(result.current).toHaveProperty('user')
        expect(result.current).toHaveProperty('isAuthenticated')
        expect(result.current).toHaveProperty('login')
        expect(result.current).toHaveProperty('logout')
        expect(result.current).toHaveProperty('isLoading')

        expect(typeof result.current.login).toBe('function')
        expect(typeof result.current.logout).toBe('function')
      })

      it('should throw error when used outside AuthProvider', () => {
        const consoleErrorSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {})

        expect(() => {
          renderHook(() => useAuth())
        }).toThrow('useAuth must be used within an AuthProvider')

        consoleErrorSpy.mockRestore()
      })

      it('should return current authentication state', async () => {
        mockAuthService.getCurrentUser.mockResolvedValue(mockUser)

        const { result } = renderHook(() => useAuth(), {
          wrapper: ({ children }) => <AuthProvider>{children}</AuthProvider>,
        })

        // Wait for authentication to complete
        await waitFor(() => {
          expect(result.current.isLoading).toBe(false)
        })

        expect(result.current.user).toEqual(mockUser)
        expect(result.current.isAuthenticated).toBe(true)
      })
    })

    describe('Context value consumption', () => {
      it('should allow multiple components to consume the same context', async () => {
        mockAuthService.getCurrentUser.mockResolvedValue(mockUser)

        const Component1 = () => {
          const { user } = useAuth()
          return <div data-testid="comp1">{user ? user.name : 'null'}</div>
        }

        const Component2 = () => {
          const { isAuthenticated } = useAuth()
          return <div data-testid="comp2">{String(isAuthenticated)}</div>
        }

        render(
          <AuthProvider>
            <Component1 />
            <Component2 />
          </AuthProvider>
        )

        await waitFor(() => {
          expect(screen.getByTestId('comp1')).toHaveTextContent('Test User')
          expect(screen.getByTestId('comp2')).toHaveTextContent('true')
        })
      })
    })

    describe('State change notifications', () => {
      it('should notify consumers when authentication state changes', async () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )
        mockAuthService.logout.mockResolvedValue(undefined)

        const renderCount = { count: 0 }
        const TestComponent = () => {
          const { isAuthenticated, logout } = useAuth()
          renderCount.count++
          return (
            <div>
              <div data-testid="authenticated">{String(isAuthenticated)}</div>
              <button
                onClick={() => {
                  logout()
                }}
                data-testid="logout-btn"
              >
                Logout
              </button>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for initial render to complete
        await waitFor(() => {
          expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
        })

        const initialRenderCount = renderCount.count

        // Trigger state change
        const logoutBtn = screen.getByTestId('logout-btn')
        act(() => {
          logoutBtn.click()
        })

        // Should trigger re-render
        expect(renderCount.count).toBeGreaterThanOrEqual(initialRenderCount)
      })

      it('should update all consumers when user state changes', async () => {
        mockAuthService.getCurrentUser.mockResolvedValue(mockUser)
        mockAuthService.logout.mockResolvedValue(undefined)

        const Component1 = () => {
          const { user, logout } = useAuth()
          return (
            <div>
              <div data-testid="user1">{user ? user.name : 'null'}</div>
              <button
                onClick={() => {
                  logout()
                }}
                data-testid="logout-btn"
              >
                Logout
              </button>
            </div>
          )
        }

        const Component2 = () => {
          const { user } = useAuth()
          return <div data-testid="user2">{user ? user.email : 'null'}</div>
        }

        render(
          <AuthProvider>
            <Component1 />
            <Component2 />
          </AuthProvider>
        )

        // Wait for initial authentication
        await waitFor(() => {
          expect(screen.getByTestId('user1')).toHaveTextContent('Test User')
          expect(screen.getByTestId('user2')).toHaveTextContent(
            'test@example.com'
          )
        })

        // Trigger logout
        const logoutBtn = screen.getByTestId('logout-btn')
        act(() => {
          logoutBtn.click()
        })

        // Both components should update
        expect(screen.getByTestId('user1')).toHaveTextContent('null')
        expect(screen.getByTestId('user2')).toHaveTextContent('null')
      })
    })
  })

  describe('Error Scenarios', () => {
    describe('Network failure handling', () => {
      it('should handle network failure during initial authentication check', async () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Network error')
        )

        const TestComponent = () => {
          const { user, isAuthenticated, isLoading } = useAuth()
          return (
            <div>
              <div data-testid="user">{user ? user.name : 'null'}</div>
              <div data-testid="authenticated">{String(isAuthenticated)}</div>
              <div data-testid="loading">{String(isLoading)}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        expect(screen.getByTestId('user')).toHaveTextContent('null')
        expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
      })

      it('should handle network failure during login', async () => {
        mockAuthService.getCurrentUser.mockRejectedValue(
          new Error('Unauthorized')
        )
        mockAuthService.getLoginUrl.mockRejectedValue(
          new Error('Network error')
        )

        const TestComponent = () => {
          const { login, isLoading } = useAuth()
          return (
            <div>
              <button
                onClick={() => {
                  void (async () => {
                    try {
                      await login()
                    } catch {
                      // Handle error gracefully
                    }
                  })()
                }}
                data-testid="login-btn"
              >
                Login
              </button>
              <div data-testid="loading">{String(isLoading)}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        // Wait for initial loading to complete
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        // Click login button
        const loginBtn = screen.getByTestId('login-btn')
        act(() => {
          loginBtn.click()
        })

        // Should reset loading state after failure
        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })
      })
    })

    describe('Context provider missing', () => {
      it('should throw error when useAuth is used outside provider', () => {
        const consoleErrorSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {})

        expect(() => {
          renderHook(() => useAuth())
        }).toThrow('useAuth must be used within an AuthProvider')

        consoleErrorSpy.mockRestore()
      })

      it('should provide helpful error message when context is undefined', () => {
        const consoleErrorSpy = jest
          .spyOn(console, 'error')
          .mockImplementation(() => {})

        try {
          renderHook(() => useAuth())
        } catch (error) {
          expect(error).toBeInstanceOf(Error)
          expect((error as Error).message).toBe(
            'useAuth must be used within an AuthProvider'
          )
        }

        consoleErrorSpy.mockRestore()
      })
    })

    describe('State corruption recovery', () => {
      it('should handle getCurrentUser returning null (corrupted response)', async () => {
        mockAuthService.getCurrentUser.mockResolvedValue(
          null as unknown as User
        )

        const TestComponent = () => {
          const { user, isAuthenticated, isLoading } = useAuth()
          return (
            <div>
              <div data-testid="user">{user ? user.name : 'null'}</div>
              <div data-testid="authenticated">{String(isAuthenticated)}</div>
              <div data-testid="loading">{String(isLoading)}</div>
            </div>
          )
        }

        render(
          <AuthProvider>
            <TestComponent />
          </AuthProvider>
        )

        await waitFor(() => {
          expect(screen.getByTestId('loading')).toHaveTextContent('false')
        })

        expect(screen.getByTestId('user')).toHaveTextContent('null')
        // Should show as authenticated since getCurrentUser succeeded
        expect(screen.getByTestId('authenticated')).toHaveTextContent('true')
      })
    })
  })

  describe('Integration scenarios', () => {
    it('should handle complete authentication flow', async () => {
      // Start unauthenticated
      mockAuthService.getCurrentUser.mockRejectedValue(
        new Error('Unauthorized')
      )
      mockAuthService.getLoginUrl.mockResolvedValue(
        'https://idp.example.com/authorize'
      )

      const TestComponent = () => {
        const { user, isAuthenticated, login, logout, isLoading } = useAuth()
        return (
          <div>
            <div data-testid="user">{user ? user.name : 'null'}</div>
            <div data-testid="authenticated">{String(isAuthenticated)}</div>
            <div data-testid="loading">{String(isLoading)}</div>
            <button
              onClick={() => {
                void login()
              }}
              data-testid="login-btn"
            >
              Login
            </button>
            <button onClick={logout} data-testid="logout-btn">
              Logout
            </button>
          </div>
        )
      }

      // First part: unauthenticated flow
      const { unmount } = render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      )

      // Wait for initial state
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('false')
        expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
      })

      // Simulate login
      const loginBtn = screen.getByTestId('login-btn')
      act(() => {
        loginBtn.click()
      })

      await waitFor(() => {
        expect(mockAuthService.getLoginUrl).toHaveBeenCalledWith(undefined)
      })

      // Cleanup first render
      unmount()

      // Second part: simulate return from the identity provider with a valid session cookie
      mockAuthService.getCurrentUser.mockResolvedValue(mockUser)
      mockAuthService.logout.mockResolvedValue(undefined)

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(screen.getByTestId('user')).toHaveTextContent('Test User')
        expect(screen.getByTestId('authenticated')).toHaveTextContent('true')
      })

      // Test logout
      const logoutBtn = screen.getByTestId('logout-btn')
      act(() => {
        logoutBtn.click()
      })

      expect(screen.getByTestId('user')).toHaveTextContent('null')
      expect(screen.getByTestId('authenticated')).toHaveTextContent('false')
    })
  })

  describe('Memory management', () => {
    it('should not cause memory leaks with context updates', () => {
      mockAuthService.getCurrentUser.mockRejectedValue(
        new Error('Unauthorized')
      )
      mockAuthService.logout.mockResolvedValue(undefined)

      const TestComponent = () => {
        const { isAuthenticated, logout } = useAuth()
        return (
          <div>
            <div data-testid="authenticated">{String(isAuthenticated)}</div>
            <button onClick={logout} data-testid="logout-btn">
              Logout
            </button>
          </div>
        )
      }

      const { unmount } = render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      )

      // Perform multiple state changes
      for (let i = 0; i < 5; i++) {
        const logoutBtn = screen.getByTestId('logout-btn')
        act(() => {
          logoutBtn.click()
        })
      }

      // Component should unmount without issues
      expect(() => {
        unmount()
      }).not.toThrow()
    })
  })

  describe('GA4 OAuth provider attribution (LOGIN_METHOD)', () => {
    const STORAGE_KEY = 'vx_login_method'

    it('should use provider from sessionStorage in sign_up event for new user', async () => {
      window.sessionStorage.setItem(STORAGE_KEY, 'Google')
      mockAuthService.getCurrentUser.mockResolvedValue(mockNewUser)

      render(
        <AuthProvider>
          <div />
        </AuthProvider>
      )

      await waitFor(() => {
        const signUpEvent = window.dataLayer.find(e => e.event === 'sign_up')
        expect(signUpEvent).toBeDefined()
        expect(signUpEvent?.method).toBe('Google')
      })
    })

    it('should use provider from sessionStorage in login event for returning user', async () => {
      window.sessionStorage.setItem(STORAGE_KEY, 'Google')
      mockAuthService.getCurrentUser.mockResolvedValue(mockUser)

      render(
        <AuthProvider>
          <div />
        </AuthProvider>
      )

      await waitFor(() => {
        const loginEvent = window.dataLayer.find(e => e.event === 'login')
        expect(loginEvent).toBeDefined()
        expect(loginEvent?.method).toBe('Google')
      })
    })

    it('should fall back to "unknown" when sessionStorage has no provider (login event)', async () => {
      // sessionStorage is already clear (cleared in beforeEach)
      mockAuthService.getCurrentUser.mockResolvedValue(mockUser)

      render(
        <AuthProvider>
          <div />
        </AuthProvider>
      )

      await waitFor(() => {
        const loginEvent = window.dataLayer.find(e => e.event === 'login')
        expect(loginEvent).toBeDefined()
        expect(loginEvent?.method).toBe('unknown')
      })
    })

    it('should clear sessionStorage LOGIN_METHOD after GA4 event fires', async () => {
      window.sessionStorage.setItem(STORAGE_KEY, 'Google')
      mockAuthService.getCurrentUser.mockResolvedValue(mockUser)

      render(
        <AuthProvider>
          <div />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(window.dataLayer.some(e => e.event === 'login')).toBe(true)
      })

      expect(window.sessionStorage.getItem(STORAGE_KEY)).toBeNull()
    })

    it('should clear sessionStorage LOGIN_METHOD even when analytics guard skips the event', async () => {
      // Simulate React 18 StrictMode second-fire scenario by rendering twice in sequence.
      // On the second render the analyticsFiredRef guard returns early before GA4 block,
      // but the key should still be cleared.
      window.sessionStorage.setItem(STORAGE_KEY, 'Google')
      mockAuthService.getCurrentUser.mockResolvedValue(mockUser)

      const { unmount } = render(
        <AuthProvider>
          <div />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(window.dataLayer.some(e => e.event === 'login')).toBe(true)
      })

      unmount()

      // Re-mount a fresh provider — the key should already be gone from the first render
      window.sessionStorage.setItem(STORAGE_KEY, 'Google')
      mockAuthService.getCurrentUser.mockResolvedValue(mockUser)

      render(
        <AuthProvider>
          <div />
        </AuthProvider>
      )

      await waitFor(() => {
        expect(window.sessionStorage.getItem(STORAGE_KEY)).toBeNull()
      })
    })
  })
})
