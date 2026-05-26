import { getApiClient } from '../lib/api'

export interface Voice {
  voice_id: string
  name: string
  preview_url?: string
  labels?: Record<string, string>
  category?: string
}

interface VoicesResponse {
  voices: Voice[]
}

function buildQuery(provider?: string): string {
  return provider ? `?provider=${encodeURIComponent(provider)}` : ''
}

export async function listVoices(provider?: string): Promise<Voice[]> {
  const res = await getApiClient().get<VoicesResponse>(`/v1/voices${buildQuery(provider)}`)
  return res.voices ?? []
}

export async function refreshVoices(provider?: string): Promise<void> {
  await getApiClient().post<{ status: string }>(`/v1/voices/refresh${buildQuery(provider)}`)
}
