import { CommonModule } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';

interface Variant {
  key: string;
  name?: string;
  weight: number;
}

interface Whitelist {
  user_id: string;
  variant_key: string;
  reason: string;
}

interface Experiment {
  id?: number;
  layer_id?: number;
  key: string;
  name: string;
  bucket_start: number;
  bucket_end: number;
  targeting: {
    countries: string[];
    require_registered: boolean;
    min_account_age_days: number;
  };
  active: boolean;
  variants: Variant[];
  whitelist?: Whitelist[];
}

interface Layer {
  id?: number;
  key: string;
  name: string;
  hash_namespace: string;
  salt: string;
  experiments: Experiment[];
}

interface UserInput {
  user_id: string;
  known: boolean;
  country: string;
  registered: boolean;
  account_age_days: number;
  persist: boolean;
  record_exposure: boolean;
}

interface TraceStep {
  stage: string;
  status: string;
  detail: string;
  hash_input?: string;
  bucket?: number;
  range_start?: number;
  range_end?: number;
  experiment_key?: string;
  variant_key?: string;
  reason_code?: string;
}

interface VariantRange {
  variant: Variant;
  start: number;
  end: number;
}

interface Decision {
  user_id: string;
  layer_key: string;
  eligible: boolean;
  allocated: boolean;
  assigned: boolean;
  exposure_saved: boolean;
  status: string;
  reason_code?: string;
  reason?: string;
  hash_input?: string;
  bucket?: number;
  experiment_key?: string;
  experiment_name?: string;
  variant_key?: string;
  source?: string;
  variant_ranges?: VariantRange[];
  trace: TraceStep[];
}

interface BatchSummary {
  total: number;
  by_status: Record<string, number>;
  by_experiment: Record<string, number>;
  by_variant: Record<string, number>;
  by_source: Record<string, number>;
  decisions: Decision[];
}

interface StatRow {
  experiment_key: string;
  experiment_name: string;
  variant_key: string;
  source: string;
  reason?: string;
  assignments: number;
  exposures: number;
}

interface Segment {
  label: string;
  start: number;
  end: number;
  color: string;
  muted?: boolean;
}

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './app.component.html',
  styleUrls: ['./app.component.css']
})
export class AppComponent implements OnInit {
  layers: Layer[] = [];
  selectedLayerKey = 'checkout';
  selectedLayer?: Layer;
  configJson = '';
  configError = '';
  configSaved = false;

  user: UserInput = {
    user_id: 'synthetic-000001',
    known: true,
    country: 'US',
    registered: true,
    account_age_days: 30,
    persist: true,
    record_exposure: true
  };
  decision?: Decision;
  evaluationError = '';
  loading = false;

  batchN = 10000;
  batchCountry = 'US';
  batch?: BatchSummary;
  batchError = '';

  stats: StatRow[] = [];
  statsError = '';

  private colors = ['#2854d6', '#7c3aed', '#0e9384', '#d97706'];

  constructor(private http: HttpClient) {}

  ngOnInit(): void {
    this.loadLayers();
  }

  loadLayers(): void {
    this.http.get<{ layers: Layer[] }>('/api/layers').subscribe({
      next: data => {
        this.layers = data.layers;
        if (!this.layers.some(l => l.key === this.selectedLayerKey) && this.layers[0]) {
          this.selectedLayerKey = this.layers[0].key;
        }
        this.selectLayer();
      },
      error: err => this.evaluationError = err.error?.error || '无法连接 Go API'
    });
  }

  selectLayer(): void {
    this.selectedLayer = this.layers.find(l => l.key === this.selectedLayerKey);
    if (this.selectedLayer) {
      this.configJson = JSON.stringify(this.stripIds(this.selectedLayer), null, 2);
      this.configError = '';
      this.configSaved = false;
      this.decision = undefined;
      this.batch = undefined;
    }
  }

  private stripIds(layer: Layer): Layer {
    return {
      key: layer.key,
      name: layer.name,
      hash_namespace: layer.hash_namespace,
      salt: layer.salt,
      experiments: layer.experiments.map(exp => ({
        key: exp.key,
        name: exp.name,
        bucket_start: exp.bucket_start,
        bucket_end: exp.bucket_end,
        targeting: exp.targeting,
        active: exp.active,
        variants: exp.variants.map(v => ({ key: v.key, name: v.name, weight: v.weight })),
        whitelist: (exp.whitelist || []).map(wl => ({
          user_id: wl.user_id,
          variant_key: wl.variant_key,
          reason: wl.reason
        }))
      }))
    };
  }

  saveConfig(): void {
    this.configError = '';
    this.configSaved = false;
    let layer: Layer;
    try {
      layer = JSON.parse(this.configJson);
    } catch (err) {
      this.configError = `JSON 解析失败：${String(err)}`;
      return;
    }
    this.http.post<Layer>('/api/layers', layer).subscribe({
      next: () => {
        this.configSaved = true;
        this.loadLayers();
      },
      error: err => this.configError = err.error?.error || '保存配置失败'
    });
  }

  evaluate(): void {
    this.evaluationError = '';
    this.decision = undefined;
    this.loading = true;
    this.http.post<Decision>(`/api/layers/${this.selectedLayerKey}/evaluate`, this.user).subscribe({
      next: d => {
        this.decision = d;
        this.loading = false;
        if (this.user.persist) this.loadStats();
      },
      error: err => {
        this.evaluationError = err.error?.error || '评估失败';
        this.loading = false;
      }
    });
  }

  simulate(): void {
    this.batchError = '';
    this.batch = undefined;
    this.http.post<BatchSummary>('/api/simulate', {
      layer_key: this.selectedLayerKey,
      n: this.batchN,
      country: this.batchCountry,
      seed: 20260929
    }).subscribe({
      next: summary => this.batch = summary,
      error: err => this.batchError = err.error?.error || '批量模拟失败'
    });
  }

  loadStats(): void {
    this.statsError = '';
    this.http.get<{ stats: StatRow[] }>('/api/stats').subscribe({
      next: d => this.stats = d.stats,
      error: err => this.statsError = err.error?.error || '统计读取失败'
    });
  }

  setSample(id: string, patch: Partial<UserInput> = {}): void {
    this.user = { ...this.user, user_id: id, ...patch };
  }

  sampleBoundary2499(): void { this.setSample('boundary-4961'); }
  sampleBoundary2500(): void { this.setSample('boundary-8281'); }
  sampleBoundary4999(): void { this.setSample('boundary-4284'); }
  sampleBoundary5000(): void { this.setSample('boundary-3792'); }
  sampleOutside(): void { this.setSample('boundary-8518'); }
  sampleUnknown(): void { this.setSample('unknown-person', { known: false, country: 'US' }); }
  sampleIneligible(): void { this.setSample('synthetic-000001', { known: true, country: 'FR', registered: true, account_age_days: 30 }); }
  sampleWhitelist(): void { this.setSample('vip-001', { known: true, country: 'FR', registered: false, account_age_days: 0 }); }

  statusText(status: string): string {
    const map: Record<string, string> = {
      assigned_bucket: '普通哈希分组',
      assigned_whitelist: '白名单覆盖',
      ineligible: '命中区间但资格拒绝',
      out_of_traffic: '未落任何实验区间',
      rejected: '身份拒绝'
    };
    return map[status] || status;
  }

  statusClass(status: string): string {
    if (status.startsWith('assigned')) return 'green';
    if (status === 'ineligible') return 'amber';
    return status === 'rejected' ? 'red' : 'gray';
  }

  stepClass(step: TraceStep): string {
    return ['reject', 'override', 'pass', 'assigned', 'hit', 'allocated'].includes(step.status) ? step.status : '';
  }

  experimentSegments(layer?: Layer): Segment[] {
    if (!layer) return [];
    const experiments = [...layer.experiments].filter(e => e.active).sort((a, b) => a.bucket_start - b.bucket_start);
    const segments: Segment[] = [];
    let cursor = 0;
    experiments.forEach((exp, i) => {
      if (exp.bucket_start > cursor) {
        segments.push({ label: `空档 ${cursor}-${exp.bucket_start - 1}`, start: cursor, end: exp.bucket_start - 1, color: '#e5e9f2', muted: true });
      }
      segments.push({ label: `${exp.key} ${exp.bucket_start}-${exp.bucket_end}`, start: exp.bucket_start, end: exp.bucket_end, color: this.colors[i % this.colors.length] });
      cursor = exp.bucket_end + 1;
    });
    if (cursor < 10000) {
      segments.push({ label: `空档 ${cursor}-9999`, start: cursor, end: 9999, color: '#e5e9f2', muted: true });
    }
    return segments;
  }

  variantSegments(decision: Decision): Segment[] {
    return (decision.variant_ranges || []).map((range, i) => ({
      label: `${range.variant.key} ${range.start}-${range.end}`,
      start: range.start,
      end: range.end,
      color: this.colors[i % this.colors.length]
    }));
  }

  left(bucket: number): number {
    return (bucket / 9999) * 100;
  }

  segmentStyle(segment: Segment): Record<string, string> {
    return {
      left: `${(segment.start / 10000) * 100}%`,
      width: `${((segment.end - segment.start + 1) / 10000) * 100}%`,
      background: segment.color,
      color: segment.muted ? '#536179' : '#fff'
    };
  }

  batchEntries(map?: Record<string, number>): { key: string; value: number }[] {
    return Object.entries(map || {}).sort((a, b) => b[1] - a[1]).map(([key, value]) => ({ key, value }));
  }

  percent(n: number): string {
    if (!this.batch) return '0%';
    return `${((n / this.batch.total) * 100).toFixed(2)}%`;
  }

  traceStage(stage: string): string {
    const map: Record<string, string> = {
      input: '输入',
      identity: '身份/资格',
      whitelist: '白名单',
      layer_bucket: '层哈希桶',
      mutex_range: '互斥层区间',
      allocation: '实验归属',
      targeting: '实验资格',
      variant_hash: '变体哈希',
      variant: '最终变体',
      exposure: '曝光记录'
    };
    return map[stage] || stage;
  }
}
