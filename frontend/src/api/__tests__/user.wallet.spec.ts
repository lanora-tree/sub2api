import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import { getWallet } from '@/api/user'

describe('user wallet api', () => {
  beforeEach(() => get.mockReset())

  it('loads only the current authenticated user wallet endpoint', async () => {
    const wallet = { user_id: 17, currency: 'CNY', balance: '12.34000000' }
    get.mockResolvedValue({ data: wallet })

    await expect(getWallet()).resolves.toEqual(wallet)
    expect(get).toHaveBeenCalledWith('/user/wallet')
  })
})
