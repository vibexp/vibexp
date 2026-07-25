import { MetadataFilter } from '@/components/metadata/MetadataFilter'
import { useMetadataCatalog } from '@/components/metadata/useMetadataCatalog'
import type {
  MetadataFilterValue,
  MetadataResourceType,
} from '@/services/metadataService'

export interface MetadataFilterFieldProps {
  /** Which resource type's metadata catalog to enumerate. */
  resourceType: MetadataResourceType
  /** Narrows the catalog to the globally selected project. */
  projectId?: string
  value: MetadataFilterValue
  onChange: (value: MetadataFilterValue) => void
  /** Accessible name for the trigger, e.g. "Filter artifacts by metadata". */
  ariaLabel?: string
  disabled?: boolean
}

/**
 * `MetadataFilter` wired to its catalog — the form every list page's filter bar
 * actually wants.
 *
 * `MetadataFilter` itself stays controlled and fetch-free so it can move into
 * the design system unchanged (#521); this is the thin adapter that owns the
 * `useMetadataCatalog` call. Without it each filter bar repeats the same twelve
 * prop lines, which is duplication three list pages would carry.
 */
export function MetadataFilterField({
  resourceType,
  projectId,
  value,
  onChange,
  ariaLabel,
  disabled,
}: Readonly<MetadataFilterFieldProps>) {
  const catalog = useMetadataCatalog({ resourceType, projectId })

  return (
    <MetadataFilter
      value={value}
      onChange={onChange}
      ariaLabel={ariaLabel}
      disabled={disabled}
      keys={catalog.keys}
      keysLoading={catalog.keysLoading}
      keysError={catalog.keysError}
      onOpenCatalog={catalog.loadKeys}
      activeKey={catalog.activeKey}
      onSelectKey={catalog.selectKey}
      values={catalog.values}
      valuesLoading={catalog.valuesLoading}
      valuesError={catalog.valuesError}
      valuesTruncated={catalog.valuesTruncated}
      valueQuery={catalog.valueQuery}
      onValueQueryChange={catalog.setValueQuery}
    />
  )
}
