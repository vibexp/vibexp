import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { notifySiteInfoSaved } from '@/features/site-info/siteInfo'
import {
  type AdminInstanceSettingsAuditParams,
  adminSettingsService,
} from '@/services/adminSettingsService'

import { TextSettingField } from '../InstanceSettingsFields'
import {
  type InstanceSettingsEmptyState,
  InstanceSettingsLayout,
} from '../InstanceSettingsLayout'
import {
  type InstanceSettingsEditorOptions,
  useInstanceSettingsEditor,
} from '../useInstanceSettingsEditor'
import {
  SITE_INFO_AUDIT_FIELDS,
  SITE_INFO_FORM_FIELDS,
  SITE_INFO_LABELS,
  type SiteInfoEnvelope,
  type SiteInfoForm,
  toSiteInfoEnvelope,
  toSiteInfoForm,
  toSiteInfoUpdate,
  validateSiteInfoForm,
} from './siteInfoForm'
import { SiteInfoLogoPreview } from './SiteInfoLogoPreview'

const NOUN = 'site info'

// Module-level so the editor's effect dependencies stay stable. A save or a
// reset tells the app (`notifySiteInfoSaved`), so the shell around this page
// rebrands without a reload.
const EDITOR_OPTIONS: InstanceSettingsEditorOptions<
  SiteInfoEnvelope,
  SiteInfoForm,
  ReturnType<typeof toSiteInfoUpdate>
> = {
  get: async () => toSiteInfoEnvelope(await adminSettingsService.getSiteInfo()),
  update: async body => {
    const stored = await adminSettingsService.updateSiteInfo(body)
    notifySiteInfoSaved()
    return toSiteInfoEnvelope(stored)
  },
  reset: async () => {
    await adminSettingsService.resetSiteInfo()
    notifySiteInfoSaved()
  },
  toForm: toSiteInfoForm,
  toUpdate: toSiteInfoUpdate,
  validate: validateSiteInfoForm,
  fields: SITE_INFO_FORM_FIELDS,
  noun: NOUN,
  resetMessage: 'Site info cleared',
}

const AUDIT = {
  fetchPage: (params: AdminInstanceSettingsAuditParams) =>
    adminSettingsService.listSiteInfoAudit(params),
  fields: SITE_INFO_AUDIT_FIELDS,
  description: "Every change to this instance's site info.",
  entryTestId: 'instance-site-info-audit-entry',
}

// Site info has nothing to fall back on: cleared means no company branding.
const EMPTY_STATE: InstanceSettingsEmptyState = {
  statusLabel: 'Not configured',
  storedLabel: 'Configured',
  resetLabel: 'Clear site info',
  resetTitle: 'Clear the site info?',
  resetDescription:
    'Every value is removed and this instance shows no company branding, only VibeXP’s own, until you enter it again.',
}

/**
 * Admin → Settings → Site info (#1358): the company branding shown across
 * the instance and in its emails. Every field is optional; an empty one
 * hides whatever would have shown it.
 */
export function AdminSiteInfoSettings() {
  const editor = useInstanceSettingsEditor(EDITOR_OPTIONS)
  const { errors, setField } = editor

  return (
    <InstanceSettingsLayout
      editor={editor}
      noun={NOUN}
      formLabel="Instance site info"
      emptyState={EMPTY_STATE}
      audit={AUDIT}
    >
      {(form, _settings, disabled) => {
        const field = (
          id: keyof SiteInfoForm,
          props: {
            type?: 'text' | 'email' | 'url'
            hint: string
            placeholder?: string
          }
        ) => (
          <TextSettingField
            id={id}
            label={SITE_INFO_LABELS[id]}
            value={form[id]}
            error={errors[id]}
            disabled={disabled}
            onChange={value => {
              setField(id, value)
            }}
            {...props}
          />
        )
        return (
          <>
            <Card>
              <CardHeader>
                <CardTitle>Company</CardTitle>
                <CardDescription>
                  Who runs this instance. Every field is optional: leave one
                  empty and whatever would show it is hidden. “Powered by
                  VibeXP” always remains.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {field('company_name', {
                  hint: 'Shown in the copyright line, as the logo’s alt text and in “Sign in to …”.',
                  placeholder: 'Acme Inc.',
                })}
                {field('company_email', {
                  type: 'email',
                  hint: 'One address. Used as the Support link when no support URL is set.',
                  placeholder: 'hello@example.com',
                })}
                {field('website', {
                  type: 'url',
                  hint: 'The logo and the company name link here.',
                  placeholder: 'https://example.com',
                })}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Links</CardTitle>
                <CardDescription>
                  Shown in the app’s footer and menus, on the sign-in page and
                  in emails.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {field('support_url', {
                  type: 'url',
                  hint: 'Where the Support link points. Without it, the company email is used; without either, there is no Support link.',
                  placeholder: 'https://example.com/help',
                })}
                {field('terms_url', {
                  type: 'url',
                  hint: 'Your terms of service page.',
                  placeholder: 'https://example.com/terms',
                })}
                {field('privacy_url', {
                  type: 'url',
                  hint: 'Your privacy policy page.',
                  placeholder: 'https://example.com/privacy',
                })}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Logo</CardTitle>
                <CardDescription>
                  Addresses of images you host. Visitors’ browsers and email
                  clients load them from that host, so it sees those requests.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {field('logo_url', {
                  type: 'url',
                  hint: 'Shown in the sidebar, on the sign-in page and in emails.',
                  placeholder: 'https://cdn.example.com/logo.svg',
                })}
                {field('logo_dark_url', {
                  type: 'url',
                  hint: 'Used in the dark theme. Without it, the logo above is used there too.',
                  placeholder: 'https://cdn.example.com/logo-dark.svg',
                })}
                <SiteInfoLogoPreview
                  logoUrl={form.logo_url}
                  logoDarkUrl={form.logo_dark_url}
                />
              </CardContent>
            </Card>
          </>
        )
      }}
    </InstanceSettingsLayout>
  )
}
