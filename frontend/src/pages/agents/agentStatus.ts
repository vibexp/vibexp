/**
 * Agent status, described the way a resource descriptor describes one.
 *
 * The table itself now lives on the `agent` descriptor (#918) — an agent is a
 * descriptor-only kind, exactly like the gallery prompt — and is re-exported
 * here so the agents list column keeps its existing import. Whichever way it is
 * reached, the agents list and the agent detail page read the one tone/label
 * table, which is the whole point of #907: one status treatment per enum, not
 * one per page.
 */
export { agentStatusField } from '@/components/patterns/resource/descriptors/agent'
