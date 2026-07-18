import { useEffect, useState } from 'react'

import type { Project } from '@/services/projectService'
import type { CreatePromptRequest, Prompt } from '@/services/promptService'

import { useTeam } from '../contexts/TeamContext'
import { projectService } from '../services/projectService'

interface PromptFormData {
  name: string
  slug: string
  description: string
  body: string
  project_id: string
  status: 'draft' | 'published'
  labels: string[]
}

interface UsePromptFormProps {
  prompt?: Prompt
  onSubmit: (data: CreatePromptRequest) => Promise<void>
}

interface UsePromptFormReturn {
  formData: PromptFormData
  setFormData: React.Dispatch<React.SetStateAction<PromptFormData>>
  labelInput: string
  setLabelInput: React.Dispatch<React.SetStateAction<string>>
  errors: Record<string, string>
  setErrors: React.Dispatch<React.SetStateAction<Record<string, string>>>
  projects: Project[]
  projectsLoading: boolean
  handleSubmit: (e: React.SubmitEvent) => Promise<void>
  handleNameChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  handleAddLabel: () => void
  handleRemoveLabel: (labelToRemove: string) => void
  handleLabelInputKeyPress: (e: React.KeyboardEvent<HTMLInputElement>) => void
  validateForm: () => boolean
}

const generateSlug = (name: string): string => {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/(^-|-$)+/g, '')
}

export function usePromptForm({
  prompt,
  onSubmit,
}: UsePromptFormProps): UsePromptFormReturn {
  const { currentTeam } = useTeam()
  const [formData, setFormData] = useState<PromptFormData>({
    name: '',
    slug: '',
    description: '',
    body: '',
    project_id: '',
    status: 'draft',
    labels: [],
  })
  const [labelInput, setLabelInput] = useState('')
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [projects, setProjects] = useState<Project[]>([])
  const [projectsLoading, setProjectsLoading] = useState(true)

  // Fetch projects on mount
  useEffect(() => {
    const fetchProjects = async () => {
      if (!currentTeam?.id) {
        setProjectsLoading(false)
        return
      }

      try {
        setProjectsLoading(true)
        const response = await projectService.getProjects(currentTeam.id, {
          limit: 100,
        })
        setProjects(response.projects)
      } catch (error) {
        console.error('Failed to fetch projects:', error)
      } finally {
        setProjectsLoading(false)
      }
    }
    void fetchProjects()
  }, [currentTeam?.id])

  // Populate form when editing existing prompt
  useEffect(() => {
    if (prompt) {
      setFormData({
        name: prompt.name,
        slug: prompt.slug,
        description: prompt.description,
        body: prompt.body,
        project_id: prompt.project_id,
        status: prompt.status,
        labels: prompt.labels ?? [],
      })
    }
  }, [prompt])

  const validateForm = (): boolean => {
    const newErrors: Record<string, string> = {}

    if (!formData.name.trim()) {
      newErrors.name = 'Name is required'
    } else if (formData.name.length > 50) {
      newErrors.name = 'Name must be 50 characters or less'
    }

    if (!formData.slug.trim()) {
      newErrors.slug = 'Slug is required'
    } else if (formData.slug.length > 255) {
      newErrors.slug = 'Slug must be 255 characters or less'
    }

    if (!formData.project_id) {
      newErrors.project_id = 'Project is required'
    }

    if (formData.description.length > 200) {
      newErrors.description = 'Description must be 200 characters or less'
    }

    if (!formData.body.trim()) {
      newErrors.body = 'Prompt content is required'
    }

    if (formData.labels.length > 10) {
      newErrors.labels = 'Cannot have more than 10 labels'
    }

    for (const label of formData.labels) {
      if (label.length > 50) {
        newErrors.labels = 'Each label must be 50 characters or less'
        break
      }
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSubmit = async (e: React.SubmitEvent): Promise<void> => {
    e.preventDefault()

    if (!validateForm()) {
      return
    }

    try {
      const submitData: CreatePromptRequest = {
        ...formData,
      }
      await onSubmit(submitData)
    } catch (error) {
      console.error('Error submitting form:', error)
    }
  }

  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>): void => {
    const name = e.target.value
    setFormData(prev => ({
      ...prev,
      name,
      slug: !prompt ? generateSlug(name) : prev.slug,
    }))
    if (errors.name) {
      setErrors(prev => ({ ...prev, name: '' }))
    }
  }

  const handleAddLabel = (): void => {
    const trimmedLabel = labelInput.trim()
    if (!trimmedLabel) return

    if (formData.labels.length >= 10) {
      setErrors(prev => ({
        ...prev,
        labels: 'Cannot have more than 10 labels',
      }))
      return
    }

    if (trimmedLabel.length > 50) {
      setErrors(prev => ({
        ...prev,
        labels: 'Label must be 50 characters or less',
      }))
      return
    }

    if (formData.labels.includes(trimmedLabel)) {
      setErrors(prev => ({ ...prev, labels: 'Label already exists' }))
      return
    }

    setFormData(prev => ({
      ...prev,
      labels: [...prev.labels, trimmedLabel],
    }))
    setLabelInput('')
    if (errors.labels) {
      setErrors(prev => ({ ...prev, labels: '' }))
    }
  }

  const handleRemoveLabel = (labelToRemove: string): void => {
    setFormData(prev => ({
      ...prev,
      labels: prev.labels.filter(label => label !== labelToRemove),
    }))
    if (errors.labels) {
      setErrors(prev => ({ ...prev, labels: '' }))
    }
  }

  const handleLabelInputKeyPress = (
    e: React.KeyboardEvent<HTMLInputElement>
  ): void => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleAddLabel()
    }
  }

  return {
    formData,
    setFormData,
    labelInput,
    setLabelInput,
    errors,
    setErrors,
    projects,
    projectsLoading,
    handleSubmit,
    handleNameChange,
    handleAddLabel,
    handleRemoveLabel,
    handleLabelInputKeyPress,
    validateForm,
  }
}
