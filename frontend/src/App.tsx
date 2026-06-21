import { HelmetProvider } from 'react-helmet-async'
import { BrowserRouter as Router, Route, Routes } from 'react-router-dom'

import { AlertContainer } from '@/components/AlertContainer'
import { SignInPage } from '@/components/auth/SignInPage'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { InvitationAcceptHandshake } from '@/components/invitations/InvitationAcceptHandshake'
import { InvitationResumeAfterAuth } from '@/components/invitations/InvitationResumeAfterAuth'
import { Layout } from '@/components/layout/Layout'
import { Root } from '@/components/layout/Root'
import { Toaster } from '@/components/ui/sonner'
import { AlertProvider } from '@/contexts/AlertContext'
import { AuthProvider } from '@/contexts/AuthContext'
import { TeamProvider } from '@/contexts/TeamContext'
import { useAuth } from '@/contexts/useAuth'
import { usePageTracking } from '@/hooks'
import { ThemeProvider } from '@/lib/theme'
import { AcceptInvitation } from '@/pages/auth/AcceptInvitation'
import { AuthCallback } from '@/pages/auth/AuthCallback'
import { SharedPrompt } from '@/pages/prompts/SharedPrompt'
import { AppRoutes } from '@/routes'

function PageTracker() {
  usePageTracking()
  return null
}

function AuthGate({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth()

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-muted-foreground text-sm">Loading…</div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return <SignInPage />
  }

  return <>{children}</>
}

function BareLayout({ children }: { children: React.ReactNode }) {
  return (
    <Root>
      {children}
      <Toaster />
    </Root>
  )
}

function MainApp() {
  return (
    <Root>
      <AuthGate>
        <InvitationResumeAfterAuth />
        <TeamProvider>
          <InvitationAcceptHandshake />
          <Layout>
            <AppRoutes />
          </Layout>
        </TeamProvider>
      </AuthGate>
      <Toaster />
    </Root>
  )
}

function App() {
  return (
    <ErrorBoundary showDialog={true}>
      <HelmetProvider>
        <AuthProvider>
          <AlertProvider>
            <ThemeProvider defaultTheme="system">
              <Router>
                <PageTracker />
                <Routes>
                  <Route
                    path="/auth/callback"
                    element={
                      <BareLayout>
                        <AuthCallback />
                      </BareLayout>
                    }
                  />
                  <Route
                    path="/invitations/accept/:token"
                    element={
                      <BareLayout>
                        <AcceptInvitation />
                      </BareLayout>
                    }
                  />
                  <Route
                    path="/shared/prompts/:token"
                    element={
                      <BareLayout>
                        <SharedPrompt />
                      </BareLayout>
                    }
                  />
                  <Route path="/*" element={<MainApp />} />
                </Routes>
                <AlertContainer />
              </Router>
            </ThemeProvider>
          </AlertProvider>
        </AuthProvider>
      </HelmetProvider>
    </ErrorBoundary>
  )
}

export default App
