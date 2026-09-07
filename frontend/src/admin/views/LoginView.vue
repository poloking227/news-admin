<script setup lang="ts"></script>

<template>
  <div class="mx-auto w-full max-w-sm py-16">
    <h1 class="text-center text-xl font-semibold text-gray-900">管理端登录</h1>
    <form class="mt-8 rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
      <label class="block text-sm font-medium text-gray-700" for="username">账号</label>
      <input
        id="username"
        class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2 text-sm outline-none focus:border-blue-500"
        type="text"
        autocomplete="username"
      />
      <label class="mt-4 block text-sm font-medium text-gray-700" for="password">密码</label>
      <input
        id="password"
        class="mt-1 w-full rounded-md border border-gray-300 px-3 py-2 text-sm outline-none focus:border-blue-500"
        type="password"
        autocomplete="current-password"
      />
      <button
        type="submit"
        class="mt-6 w-full rounded-md bg-gray-900 py-2 text-sm font-medium text-white hover:bg-gray-700"
        disabled
      >
        登录（与会话模块一同落地）
      </button>
    </form>
  </div>
</template>