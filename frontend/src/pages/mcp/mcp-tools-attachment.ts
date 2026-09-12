import type { MCPTool } from './mcp-tool-shared'
import { TEAM_ID_DESCRIPTION } from './mcp-tool-shared'

/**
 * The attachment tools, kept in their own module so the main catalog stays
 * under the 600-line cap the lint config enforces -- the same split
 * mcp-tools-memory.ts made for the memory write tools.
 */
export const attachmentTools: MCPTool[] = [
  {
    name: 'vibexp_io_upload_attachment',
    description:
      'Upload a base64-encoded file and attach it to a resource, keyed by owner_type and owner_id. Use this to keep a diagram, dataset or script alongside the artifact that explains it, instead of pasting it into the body. Max 5 MB per file and 10 MB total per owner.',
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        owner_type: {
          type: 'string',
          description:
            'The attachable resource type the file belongs to, e.g. "artifact". Must be a supported owner type.',
        },
        owner_id: {
          type: 'string',
          description:
            'UUID of the owning resource (e.g. the artifact id) the file is attached to.',
        },
        file_name: {
          type: 'string',
          description:
            'File name including extension, e.g. "report.pdf". The extension must be in the allowlist (png, jpg, jpeg, gif, webp, pdf, txt, md, csv, json, docx, xlsx, zip).',
        },
        file_content_base64: {
          type: 'string',
          description:
            'The file content as a standard base64-encoded string. Decoded size must not exceed 5 MB per file (10 MB total per owner).',
        },
        relative_path: {
          type: 'string',
          description:
            'Optional path relative to the owner\'s directory, e.g. "scripts/helper.py" for a multi-file skill companion. Must be relative: no leading "/", no "..", no backslashes. Unique per owner; file_name stays the basename.',
        },
      },
      required: [
        'team_id',
        'owner_type',
        'owner_id',
        'file_name',
        'file_content_base64',
      ],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_list_attachments',
    description:
      'List the attachments for a resource — metadata plus a download URL for each — keyed by owner_type and owner_id. Call this before uploading to see what is already attached, and to get the id an attachment must be deleted by.',
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        owner_type: {
          type: 'string',
          description:
            'The attachable resource type to list attachments for, e.g. "artifact".',
        },
        owner_id: {
          type: 'string',
          description:
            'UUID of the owning resource whose attachments should be listed.',
        },
      },
      required: ['team_id', 'owner_type', 'owner_id'],
      additionalProperties: false,
    },
  },
  {
    name: 'vibexp_io_delete_attachment',
    description:
      'Delete an attachment by its id. The owning resource is resolved from the stored row and authorized before deletion, so only an attachment your team owns can be removed. Call vibexp_io_list_attachments first to find the id.',
    inputSchema: {
      type: 'object',
      properties: {
        team_id: {
          type: 'string',
          description: TEAM_ID_DESCRIPTION,
        },
        attachment_id: {
          type: 'string',
          description: 'UUID of the attachment to delete.',
        },
      },
      required: ['team_id', 'attachment_id'],
      additionalProperties: false,
    },
  },
]
