import { zodResolver } from '@hookform/resolvers/zod'
import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import type {
  CreateFeedRequest,
  Feed,
  UpdateFeedRequest,
} from '@/services/feedService'

const schema = z.object({
  name: z.string().trim().min(1, 'Name is required').max(255),
  description: z.string().max(1000).optional(),
})

type FeedFormValues = z.infer<typeof schema>

export interface FeedFormHandle {
  submit: () => void
}

interface FeedFormProps {
  feed?: Feed
  onSubmit: (data: CreateFeedRequest | UpdateFeedRequest) => Promise<void>
  isLoading?: boolean
}

export const FeedForm = forwardRef<FeedFormHandle, FeedFormProps>(
  function FeedForm({ feed, onSubmit, isLoading = false }, ref) {
    const formElRef = useRef<HTMLFormElement>(null)

    const form = useForm<FeedFormValues>({
      resolver: zodResolver(schema),
      defaultValues: {
        name: feed?.name ?? '',
        description: feed?.description ?? '',
      },
    })

    useEffect(() => {
      if (feed) {
        form.reset({
          name: feed.name,
          description: feed.description ?? '',
        })
      }
    }, [feed, form])

    useImperativeHandle(ref, () => ({
      submit() {
        formElRef.current?.requestSubmit()
      },
    }))

    const handleSubmit = form.handleSubmit(async values => {
      await onSubmit({
        name: values.name.trim(),
        description:
          values.description?.trim() !== ''
            ? values.description?.trim()
            : undefined,
      })
    })

    return (
      <Form {...form}>
        <form
          ref={formElRef}
          onSubmit={event => {
            void handleSubmit(event)
          }}
        >
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Feed details</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Name</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        disabled={isLoading}
                        placeholder="e.g. Product Updates"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="description"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Description</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        disabled={isLoading}
                        rows={4}
                        placeholder="Optional description of this feed's purpose"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </CardContent>
          </Card>
        </form>
      </Form>
    )
  }
)
