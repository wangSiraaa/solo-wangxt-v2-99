import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';

import { ExperimentConfig, Layer, Variant } from './models';
import { ExperimentService } from './experiment.service';

@Component({
  selector: 'app-config',
  standalone: true,
  imports: [FormsModule],
  template: `
    <section class="card">
      <h2>互斥层配置</h2>
      <p class="muted">命名空间 + 层 ID + 层盐值 + 用户 ID 参与第一层哈希；实验 ID + 实验盐值再参与变体哈希。</p>
      @if (config(); as cfg) {
        @for (layer of cfg.layers; track layer.id) {
          <div class="config-block">
            <h3>{{layer.id}}</h3>
            <label>namespace<input [(ngModel)]="layer.namespace" /></label>
            <label>name<input [(ngModel)]="layer.name" /></label>
            <label>layer salt<input [(ngModel)]="layer.salt" /></label>
            <label>bucket size<input type="number" [(ngModel)]="layer.bucket_size" /></label>
            <button (click)="saveLayer(layer)">保存层</button>
          </div>
        }
      }
    </section>

    <section class="card">
      <h2>实验与桶区间</h2>
      @if (config(); as cfg) {
        @for (exp of cfg.experiments; track exp.id) {
          <div class="config-block">
            <h3>{{exp.id}}</h3>
            <p class="muted">层：{{exp.layer_id}}；区间必须半开且互斥：[start, end)。</p>
            <label>name<input [(ngModel)]="exp.name" /></label>
            <div class="form-row">
              <label>start<input type="number" [(ngModel)]="exp.start_bucket" /></label>
              <label>end<input type="number" [(ngModel)]="exp.end_bucket" /></label>
            </div>
            <label>variant salt<input [(ngModel)]="exp.salt" /></label>
            <label>允许国家（逗号分隔）
              <input [ngModel]="exp.criteria.allowed_countries.join(',')"
                     (ngModelChange)="exp.criteria.allowed_countries = splitCsv($event)" />
            </label>
            <label>允许套餐（逗号分隔）
              <input [ngModel]="exp.criteria.allowed_plans.join(',')"
                     (ngModelChange)="exp.criteria.allowed_plans = splitCsv($event)" />
            </label>
            <label class="checkbox"><input type="checkbox" [(ngModel)]="exp.criteria.require_registered" />要求已注册</label>
            <button (click)="saveExperiment(exp)">保存实验</button>
          </div>
        }
      }
    </section>

    <section class="card">
      <h2>变体桶区间</h2>
      @if (config(); as cfg) {
        @for (variant of cfg.variants; track variant.id) {
          <div class="config-block">
            <h3>{{variant.id}}</h3>
            <p class="muted">实验：{{variant.experiment_id}}；变体区间同样使用 [start, end)。</p>
            <label>name<input [(ngModel)]="variant.name" /></label>
            <div class="form-row">
              <label>start<input type="number" [(ngModel)]="variant.start_bucket" /></label>
              <label>end<input type="number" [(ngModel)]="variant.end_bucket" /></label>
            </div>
            <label class="checkbox"><input type="checkbox" [(ngModel)]="variant.active" />启用</label>
            <button (click)="saveVariant(variant)">保存变体</button>
          </div>
        }
      }
    </section>

    @if (message()) {<div class="alert">{{message()}}</div>}
  `
})
export class ConfigComponent implements OnInit {
  private readonly service = inject(ExperimentService);
  readonly config = signal<ExperimentConfig | null>(null);
  readonly message = signal('');

  async ngOnInit(): Promise<void> {
    this.config.set(await firstValueFrom(this.service.config()));
  }

  splitCsv(value: string): string[] {
    return value.split(',').map(s => s.trim()).filter(Boolean);
  }

  async saveLayer(layer: Layer): Promise<void> {
    await firstValueFrom(this.service.saveLayer(layer));
    this.message.set('层配置已保存');
  }

  async saveExperiment(experiment: ExperimentConfig['experiments'][number]): Promise<void> {
    await firstValueFrom(this.service.saveExperiment(experiment));
    this.message.set('实验配置已保存');
  }

  async saveVariant(variant: Variant): Promise<void> {
    await firstValueFrom(this.service.saveVariant(variant));
    this.message.set('变体配置已保存');
  }
}
