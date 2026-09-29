import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

import {
  DecideResponse,
  ExperimentConfig,
  Layer,
  SimulationResponse,
  Stats,
  Subject,
  Variant,
  Whitelist,
  Experiment
} from './models';

@Injectable({ providedIn: 'root' })
export class ExperimentService {
  private readonly http = inject(HttpClient);
  private readonly api = '/api';

  config(): Observable<ExperimentConfig> {
    return this.http.get<ExperimentConfig>(`${this.api}/config`);
  }

  decide(subject: Subject, layerId: string, record: boolean, expose: boolean): Observable<DecideResponse> {
    return this.http.post<DecideResponse>(`${this.api}/decide`, {
      subject,
      layer_id: layerId,
      record,
      expose
    });
  }

  simulate(count: number): Observable<SimulationResponse> {
    return this.http.post<SimulationResponse>(`${this.api}/simulate`, { count });
  }

  stats(source = 'synthetic'): Observable<Stats> {
    return this.http.get<Stats>(`${this.api}/stats`, { params: { source } });
  }

  saveLayer(layer: Layer) {
    return this.http.put(`${this.api}/config/layers/${layer.id}`, layer);
  }

  saveExperiment(experiment: Experiment) {
    return this.http.put(`${this.api}/config/experiments/${experiment.id}`, experiment);
  }

  saveVariant(variant: Variant) {
    return this.http.put(`${this.api}/config/variants/${variant.id}`, variant);
  }

  saveWhitelist(whitelist: Whitelist) {
    return this.http.put(`${this.api}/config/whitelists/${whitelist.id}`, whitelist);
  }
}
