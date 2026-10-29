<script setup lang="ts">
import MailPane from '@/components/MailPane.vue'
</script>

<template>
  <div class="flex min-h-[32rem] flex-col lg:h-full lg:min-h-0">
    <h2 class="mb-4 shrink-0 text-xl font-semibold">收件箱</h2>
    <MailPane class="lg:min-h-0 lg:flex-1" base="/mails" empty-text="还没有邮件" />
  </div>
</template>
