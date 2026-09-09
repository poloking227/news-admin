import { describe, expect, it } from 'vitest'
import { ApiError, parseErrorEnvelope } from '@/shared/api/http'

describe('parseErrorEnvelope', () => {
  it('解析契约格式的错误信封', () => {
    const body = parseErrorEnvelope({
      error: { code: 'VALIDATION_FAILED', message: '字段非法', details: { username: '必填' } },
    })
    expect(body).toEqual({
      code: 'VALIDATION_FAILED',
      message: '字段非法',
      details: { username: '必填' },
    })
  })

  it('details 缺省时保持 undefined', () => {
    const body = parseErrorEnvelope({ error: { code: 'NOT_FOUND', message: '不存在' } })
    expect(body.details).toBeUndefined()
  })

  it('对不符合信封结构的载荷兜底为 INTERNAL_ERROR', () => {
    const body = parseErrorEnvelope({ message: 'no wrapper' })
    expect(body.code).toBe('INTERNAL_ERROR')
    expect(body.message).toContain('no wrapper')
  })

  it('对 null / 非对象载荷兜底', () => {
    expect(parseErrorEnvelope(null).code).toBe('INTERNAL_ERROR')
    expect(parseErrorEnvelope('oops').code).toBe('INTERNAL_ERROR')
  })
})

describe('ApiError', () => {
  it('携带稳定 code、message、status 与信封还原', () => {
    const err = new ApiError(
      { code: 'RATE_LIMITED', message: '请求过于频繁', details: { retryAfter: 60 } },
      429,
    )
    expect(err).toBeInstanceOf(Error)
    expect(err.code).toBe('RATE_LIMITED')
    expect(err.message).toBe('请求过于频繁')
    expect(err.status).toBe(429)
    expect(err.envelope).toEqual({
      error: { code: 'RATE_LIMITED', message: '请求过于频繁', details: { retryAfter: 60 } },
    })
  })

  it('无 details 时信封不含 details 字段', () => {
    const err = new ApiError({ code: 'FORBIDDEN', message: '权限不足' }, 403)
    expect(err.envelope).toEqual({ error: { code: 'FORBIDDEN', message: '权限不足' } })
  })
})