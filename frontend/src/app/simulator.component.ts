import { DecimalPipe, JsonPipe } from '@angular/common';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';

import {
  DecideResponse,
  ExperimentConfig,
  SimulationResponse,
  Stats
} from './models';
import { ExperimentService } from './experiment.service';

@Component({
  selector: 'app-simulator',
  standalone: true,
  imports: [FormsModule, JsonPipe, DecimalPipe],
  templateUrl: './simulator.component.html'
})
export class SimulatorComponent implements OnInit {
  private readonly service = inject(ExperimentService);

  config = signal<ExperimentConfig | null>(null);
  result = signal<DecideResponse | null>(null);
  simulation = signal<SimulationResponse | null>(null);
  stats = signal<Stats | null>(null);
  error = signal('');
  loading = signal(false);

  userId = 'syn-3340';
  registered = true;
  country = 'US';
  plan = 'free';
  source = 'synthetic';
  layerId = 'checkout_layer';
  record = false;
  expose = false;
  batchCount = 1000;

  async ngOnInit(): Promise<void> {
    await this.refresh();
  }

  useSample(userId: string, registered: boolean, country: string, plan: string): void {
    this.userId = userId;
    this.registered = registered;
    this.country = country;
    this.plan = plan;
    this.record = false;
    this.expose = false;
    this.result.set(null);
  }

  async decide(): Promise<void> {
    this.loading.set(true);
    this.error.set('');
    try {
      const response = await firstValueFrom(this.service.decide(
        {
          user_id: this.userId.trim(),
          registered: this.registered,
          country: this.country,
          plan: this.plan,
          source: this.source || 'synthetic'
        },
        this.layerId,
        this.record,
        this.expose
      ));
      this.result.set(response);
      if (this.record) {
        await this.loadStats();
      }
    } catch (err: unknown) {
      this.error.set(this.message(err));
    } finally {
      this.loading.set(false);
    }
  }

  async simulate(): Promise<void> {
    this.loading.set(true);
    this.error.set('');
    try {
      this.simulation.set(await firstValueFrom(this.service.simulate(this.batchCount)));
    } catch (err: unknown) {
      this.error.set(this.message(err));
    } finally {
      this.loading.set(false);
    }
  }

  async refresh(): Promise<void> {
    this.loading.set(true);
    this.error.set('');
    try {
      this.config.set(await firstValueFrom(this.service.config()));
      await this.loadStats();
    } catch (err: unknown) {
      this.error.set(this.message(err));
    } finally {
      this.loading.set(false);
    }
  }

  private async loadStats(): Promise<void> {
    this.stats.set(await firstValueFrom(this.service.stats('synthetic')));
  }

  entries(map: Record<string, number> | null | undefined): Array<[string, number]> {
    return Object.entries(map ?? {}).sort(([a], [b]) => a.localeCompare(b));
  }

  private message(err: unknown): string {
    const maybe = err as { error?: { error?: string }; message?: string };
    return maybe.error?.error ?? maybe.message ?? String(err);
  }
}
