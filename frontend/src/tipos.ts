// Tipos que reflejan el contrato JSON del dispatcher (Go -> TypeScript).

export type Estado = 'VIVA' | 'CAIDA'

export interface SaludReplica {
  replica_id: string
  url_base: string
  estado: Estado
  fallos_consecutivos: number
  exitos_consecutivos: number
  total_sondeos: number
  total_fallos: number
  ultimo_sondeo?: string
  ultimo_cambio?: string
  ultima_latencia_ms: number
}

export interface ConfigMonitor {
  T_intervalo_ms: number
  t_timeout_ms: number
  k_fallos_para_caida: number
  m_exitos_para_viva: number
  timeout_consulta_ms: number
  deteccion_esperada_ms: number
}

export interface Metricas {
  total: number
  exitosas: number
  fallidas: number
  ganadoras: Record<string, number>
}

export interface EstadoDetalle {
  momento: string
  replicas: SaludReplica[]
  config: ConfigMonitor
  metricas: Metricas
  bitacora: string[]
}
