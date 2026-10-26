<script setup lang="ts">
/**
 * Follow one background job to completion.
 *
 *     <JobProgress :job-id="id" @done="refresh" />
 *
 * Drop it in wherever a command returned a job id. It stops polling the moment
 * the job finishes - a completed job polled forever is a request every two
 * seconds, per open tab, indefinitely.
 */
const props = defineProps<{ jobId: string }>()
const emit = defineEmits<{ done: [result: Record<string, unknown>], failed: [reason: string] }>()

const { job, error } = useProgress(props.jobId)
const { t } = useI18n()

watch(job, (current) => {
  if (current?.status === 'done') emit('done', current.result)
  if (current?.status === 'failed') emit('failed', current.failureReason ?? '')
})
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="job-progress">
    <UiProgressBar
      v-if="job"
      :percent="job.percent"
      :tone="job.status === 'failed' ? 'danger' : job.status === 'done' ? 'success' : 'brand'"
      :label="job.status === 'running'
        ? t('progress.running', { done: job.done, total: job.total })
        : job.status === 'done'
          ? t('progress.done')
          : t('progress.failed', { reason: job.failureReason ?? '' })"
    />

    <p v-if="error" class="text-xs text-danger" role="alert">{{ error }}</p>
  </div>
</template>
