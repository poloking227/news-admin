<script setup lang="ts">
import { EditorContent, useEditor } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import { watch } from 'vue'

const props = defineProps<{
  /** 编辑器 HTML 内容（双向 v-model） */
  modelValue: string
  placeholder?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const editor = useEditor({
  content: props.modelValue,
  extensions: [StarterKit],
  editorProps: {
    attributes: {
      class: 'min-h-[220px] px-3 py-2 focus:outline-none'
    }
  },
  onUpdate: ({ editor }) => {
    emit('update:modelValue', editor.getHTML())
  }
})

// 外部内容变化（如编辑对话框回填）时同步编辑器，避免覆盖光标态
watch(
  () => props.modelValue,
  (value) => {
    if (editor.value && value !== editor.value.getHTML()) {
      editor.value.commands.setContent(value || '')
    }
  }
)

function isActive(name: string, attributes?: Record<string, unknown>): boolean {
  return editor.value?.isActive(name, attributes) ?? false
}

function toggleMark(name: 'bold' | 'italic' | 'strike' | 'code'): void {
  editor.value?.chain().focus().toggleMark(name).run()
}

function toggleNode(
  name: 'heading' | 'bulletList' | 'orderedList' | 'blockquote' | 'codeBlock'
): void {
  editor.value?.chain().focus().toggleNode(name, 'paragraph').run()
}

function setHeading(level: 2 | 3): void {
  editor.value?.chain().focus().toggleHeading({ level }).run()
}

defineExpose({
  /** 提取纯文本（供 bodyText 字段，服务端搜索用） */
  getText: () => editor.value?.getText() ?? '',
  isEmpty: () => editor.value?.isEmpty ?? true
})
</script>

<template>
  <div
    class="overflow-hidden rounded-md border border-gray-300 focus-within:border-blue-500"
    data-testid="tiptap-editor"
  >
    <div class="flex flex-wrap gap-1 border-b border-gray-200 bg-gray-50 px-2 py-1">
      <el-button
        link
        size="small"
        :type="isActive('bold') ? 'primary' : 'default'"
        @click="toggleMark('bold')"
      >
        加粗
      </el-button>
      <el-button
        link
        size="small"
        :type="isActive('italic') ? 'primary' : 'default'"
        @click="toggleMark('italic')"
      >
        斜体
      </el-button>
      <el-button
        link
        size="small"
        :type="isActive('heading', { level: 2 }) ? 'primary' : 'default'"
        @click="setHeading(2)"
      >
        标题
      </el-button>
      <el-button
        link
        size="small"
        :type="isActive('bulletList') ? 'primary' : 'default'"
        @click="toggleNode('bulletList')"
      >
        列表
      </el-button>
      <el-button
        link
        size="small"
        :type="isActive('orderedList') ? 'primary' : 'default'"
        @click="toggleNode('orderedList')"
      >
        有序列表
      </el-button>
      <el-button
        link
        size="small"
        :type="isActive('blockquote') ? 'primary' : 'default'"
        @click="toggleNode('blockquote')"
      >
        引用
      </el-button>
    </div>
    <EditorContent :editor="editor" />
  </div>
</template>
