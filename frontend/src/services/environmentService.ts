/**
 * Environment Service
 * Provides methods to detect and manage different environments
 */

class EnvironmentService {
  /**
   * Checks if the application is running in development mode
   * Uses Vite's import.meta.env.DEV and hostname checks
   */
  isDevelopment(): boolean {
    // Check Vite's built-in environment variable
    if (import.meta.env.DEV) {
      return true
    }

    // Also check hostname as a fallback
    const hostname = window.location.hostname.toLowerCase()
    return hostname === 'localhost' || hostname === '127.0.0.1'
  }

  /**
   * Checks if the application is running in production mode
   */
  isProduction(): boolean {
    return import.meta.env.PROD && !this.isStaging()
  }

  /**
   * Checks if the application is running in staging mode
   * Looks for "staging" in the hostname
   */
  isStaging(): boolean {
    const hostname = window.location.hostname.toLowerCase()
    return hostname.includes('staging')
  }

  /**
   * Returns the current environment name
   */
  getEnvironmentName(): 'development' | 'staging' | 'production' {
    if (this.isDevelopment()) {
      return 'development'
    }
    if (this.isStaging()) {
      return 'staging'
    }
    return 'production'
  }

  /**
   * Determines if dev login should be enabled
   * Dev login is only active in development environment
   */
  isDevLoginEnabled(): boolean {
    return this.isDevelopment()
  }
}

// Export singleton instance
export const environmentService = new EnvironmentService()
export default environmentService
