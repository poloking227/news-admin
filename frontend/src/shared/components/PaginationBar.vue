<script setup lang="ts">
const props = defineProps<{
  total: number
  page: number
  pageSize: number
  pageSizes?: number[]
}>()

const emit = defineEmits<{
  'update:page': [value: number]
  'update:pageSize': [value: number]
  /** 页码/每页条数变化后的查询事件（父组件据此发起请求） */
  change: []
}>()

const sizes = props.pageSizes ?? [10, 20, 50]

function onCurrentChange(page: number) {
  emit('update:page', page)
  emit('change')
}

function onSizeChange() {
  emit('update:page', 1)
  emit('change')
}
</script>

<template>
  <div class="flex justify-end">
    <el-pagination
      :total="total"
      :current-page="page"
      :page-size="pageSize"
      :page-sizes="sizes"
      layout="total, sizes, prev, pager, next"
      @current-change="onCurrentChange"
      @size-change="onSizeChange"
    />
  </div>
</template>
