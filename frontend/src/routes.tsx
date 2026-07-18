import { Route, Routes } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { Activities } from '@/pages/activities/Activities'
import { AgentChat } from '@/pages/agents/AgentChat'
import { AgentConversations } from '@/pages/agents/AgentConversations'
import { AgentDetails } from '@/pages/agents/AgentDetails'
import { AgentEditor } from '@/pages/agents/AgentEditor'
import { Agents } from '@/pages/agents/Agents'
import { AgentTasks } from '@/pages/agents/AgentTasks'
import { AIToolsOverview } from '@/pages/ai-tools/AIToolsOverview'
import { ClaudeCodeOverview } from '@/pages/ai-tools/claude-code/ClaudeCodeOverview'
import { CursorIDEOverview } from '@/pages/ai-tools/cursor-ide/CursorIDEOverview'
import { DeferredToolPage } from '@/pages/ai-tools/DeferredToolPage'
import { ArtifactCreate } from '@/pages/artifacts/ArtifactCreate'
import { ArtifactEdit } from '@/pages/artifacts/ArtifactEdit'
import { Artifacts } from '@/pages/artifacts/Artifacts'
import { ArtifactVersions } from '@/pages/artifacts/ArtifactVersions'
import { ArtifactView } from '@/pages/artifacts/ArtifactView'
import { BlueprintCreate } from '@/pages/blueprints/BlueprintCreate'
import { BlueprintEdit } from '@/pages/blueprints/BlueprintEdit'
import { Blueprints } from '@/pages/blueprints/Blueprints'
import { BlueprintVersions } from '@/pages/blueprints/BlueprintVersions'
import { BlueprintView } from '@/pages/blueprints/BlueprintView'
import { NotFound } from '@/pages/errors/NotFound'
import { FeedEdit } from '@/pages/feeds/FeedEdit'
import { FeedItemView } from '@/pages/feeds/FeedItemView'
import { FeedNew } from '@/pages/feeds/FeedNew'
import { Feeds } from '@/pages/feeds/Feeds'
import { FeedView } from '@/pages/feeds/FeedView'
import { Home } from '@/pages/home/Home'
import { VibeXPMCP } from '@/pages/mcp/VibeXPMCP'
import { Memories } from '@/pages/memories/Memories'
import { MemoryCreate } from '@/pages/memories/MemoryCreate'
import { MemoryEdit } from '@/pages/memories/MemoryEdit'
import { MemoryVersions } from '@/pages/memories/MemoryVersions'
import { MemoryView } from '@/pages/memories/MemoryView'
import { Notifications } from '@/pages/notifications/Notifications'
import { PromptGallery } from '@/pages/prompt-gallery/PromptGallery'
import { PromptGalleryCategory } from '@/pages/prompt-gallery/PromptGalleryCategory'
import { PromptGalleryDetail } from '@/pages/prompt-gallery/PromptGalleryDetail'
import { PromptDetail } from '@/pages/prompts/PromptDetail'
import { PromptEditor } from '@/pages/prompts/PromptEditor'
import { Prompts } from '@/pages/prompts/Prompts'
import { PromptVersions } from '@/pages/prompts/PromptVersions'
import { Search } from '@/pages/search/Search'
import { APIKeys } from '@/pages/settings/api-keys/APIKeys'
import { Customization } from '@/pages/settings/customization/Customization'
import { EmbeddingProviders } from '@/pages/settings/embedding-providers/EmbeddingProviders'
import { GitHubIntegration } from '@/pages/settings/integrations/github/GitHubIntegration'
import { ModelProviders } from '@/pages/settings/model-providers/ModelProviders'
import { NotificationPreferences } from '@/pages/settings/notifications/NotificationPreferences'
import { ProjectCreate } from '@/pages/settings/projects/ProjectCreate'
import { ProjectDetails } from '@/pages/settings/projects/ProjectDetails'
import { ProjectEdit } from '@/pages/settings/projects/ProjectEdit'
import { ProjectMigrate } from '@/pages/settings/projects/ProjectMigrate'
import { Projects } from '@/pages/settings/projects/Projects'
import { Settings } from '@/pages/settings/Settings'
import { TeamAnalyticsPage } from '@/pages/settings/teams/TeamAnalyticsPage'
import { TeamDetailsPage } from '@/pages/settings/teams/TeamDetailsPage'
import { Teams } from '@/pages/settings/teams/Teams'
import { Showcase } from '@/pages/Showcase'

function ComingSoon({ title }: Readonly<{ title: string }>) {
  return (
    <div className="mx-auto max-w-4xl">
      <PageHeader title={title} description="This page is coming soon." />
    </div>
  )
}

export function AppRoutes() {
  return (
    <Routes>
      <Route index element={<Home />} />
      <Route path="search" element={<Search />} />
      <Route path="showcase" element={<Showcase />} />
      <Route path="prompts" element={<Prompts />} />
      <Route path="prompts/new" element={<PromptEditor />} />
      <Route path="prompts/:slug" element={<PromptDetail />} />
      <Route path="prompts/:slug/edit" element={<PromptEditor />} />
      <Route path="prompts/:slug/versions" element={<PromptVersions />} />
      <Route path="prompt-gallery" element={<PromptGallery />} />
      <Route
        path="prompt-gallery/prompt/:id"
        element={<PromptGalleryDetail />}
      />
      <Route
        path="prompt-gallery/:category"
        element={<PromptGalleryCategory />}
      />
      <Route path="artifacts" element={<Artifacts />} />
      <Route path="artifacts/new" element={<ArtifactCreate />} />
      <Route path="artifacts/:project/:slug" element={<ArtifactView />} />
      <Route path="artifacts/:project/:slug/edit" element={<ArtifactEdit />} />
      <Route
        path="artifacts/:project/:slug/versions"
        element={<ArtifactVersions />}
      />
      <Route path="blueprints" element={<Blueprints />} />
      <Route path="blueprints/new" element={<BlueprintCreate />} />
      <Route path="blueprints/:project/:slug" element={<BlueprintView />} />
      <Route
        path="blueprints/:project/:slug/edit"
        element={<BlueprintEdit />}
      />
      <Route
        path="blueprints/:project/:slug/versions"
        element={<BlueprintVersions />}
      />
      <Route path="feeds" element={<Feeds />} />
      <Route path="feeds/new" element={<FeedNew />} />
      <Route path="feeds/:feedId" element={<FeedView />} />
      <Route path="feeds/:feedId/edit" element={<FeedEdit />} />
      <Route path="feed-items/:itemId" element={<FeedItemView />} />
      <Route path="memories" element={<Memories />} />
      <Route path="memories/new" element={<MemoryCreate />} />
      <Route path="memories/:id" element={<MemoryView />} />
      <Route path="memories/:id/edit" element={<MemoryEdit />} />
      <Route path="memories/:id/versions" element={<MemoryVersions />} />
      <Route path="agents" element={<Agents />} />
      <Route path="agents/add" element={<AgentEditor />} />
      <Route path="agents/:id" element={<AgentDetails />} />
      <Route path="agents/:id/edit" element={<AgentEditor />} />
      <Route path="agents/:id/chat" element={<AgentChat />} />
      <Route path="agents/:id/conversations" element={<AgentConversations />} />
      <Route path="agents/:id/tasks" element={<AgentTasks />} />
      <Route path="ai-tools/overview" element={<AIToolsOverview />} />
      <Route
        path="ai-tools/claude-code/overview"
        element={<ClaudeCodeOverview />}
      />
      <Route
        path="ai-tools/claude-code/setup"
        element={
          <DeferredToolPage
            title="Claude Code — Setup"
            description="Configure Claude Code hooks and integration with VibeXP."
            backHref="/ai-tools/claude-code/overview"
          />
        }
      />
      <Route
        path="ai-tools/claude-code/sessions"
        element={
          <DeferredToolPage
            title="Claude Code — Sessions"
            description="Browse your recorded Claude Code sessions."
            backHref="/ai-tools/claude-code/overview"
          />
        }
      />
      <Route
        path="ai-tools/claude-code/session/:sessionId"
        element={
          <DeferredToolPage
            title="Claude Code — Session detail"
            description="Inspect a single Claude Code session."
            backHref="/ai-tools/claude-code/overview"
          />
        }
      />
      <Route
        path="ai-tools/cursor-ide/overview"
        element={<CursorIDEOverview />}
      />
      <Route
        path="ai-tools/cursor-ide/setup"
        element={
          <DeferredToolPage
            title="Cursor IDE — Setup"
            description="Configure Cursor IDE integration with VibeXP."
            backHref="/ai-tools/cursor-ide/overview"
          />
        }
      />
      <Route
        path="ai-tools/cursor-ide/sessions"
        element={
          <DeferredToolPage
            title="Cursor IDE — Sessions"
            description="Browse your recorded Cursor IDE sessions."
            backHref="/ai-tools/cursor-ide/overview"
          />
        }
      />
      <Route
        path="ai-tools/cursor-ide/session/:sessionId"
        element={
          <DeferredToolPage
            title="Cursor IDE — Session detail"
            description="Inspect a single Cursor IDE session."
            backHref="/ai-tools/cursor-ide/overview"
          />
        }
      />
      <Route path="ai-tools/*" element={<ComingSoon title="AI Tools" />} />
      <Route path="mcp-servers/vibexp-mcp" element={<VibeXPMCP />} />
      <Route
        path="mcp-servers/*"
        element={<ComingSoon title="MCP Servers" />}
      />
      <Route path="notifications" element={<Notifications />} />
      <Route path="settings" element={<Settings />} />
      <Route path="settings/activities" element={<Activities />} />
      <Route
        path="settings/notifications"
        element={<NotificationPreferences />}
      />
      <Route path="settings/api-keys" element={<APIKeys />} />
      <Route path="settings/customization" element={<Customization />} />
      <Route
        path="settings/embedding-providers"
        element={<EmbeddingProviders />}
      />
      <Route path="settings/model-providers" element={<ModelProviders />} />
      <Route path="settings/projects" element={<Projects />} />
      <Route path="settings/projects/create" element={<ProjectCreate />} />
      <Route path="settings/projects/edit/:slug" element={<ProjectEdit />} />
      <Route
        path="settings/projects/:slug/migrate"
        element={<ProjectMigrate />}
      />
      <Route path="settings/projects/:slug" element={<ProjectDetails />} />
      <Route
        path="settings/integrations/github"
        element={<GitHubIntegration />}
      />
      <Route path="settings/teams" element={<Teams />} />
      <Route path="settings/teams/:id" element={<TeamDetailsPage />} />
      <Route
        path="settings/teams/:id/analytics"
        element={<TeamAnalyticsPage />}
      />
      <Route path="settings/*" element={<ComingSoon title="Settings" />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}
