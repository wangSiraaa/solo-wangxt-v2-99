import { bootstrapApplication } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { provideRouter } from '@angular/router';

import { AppComponent } from './app/app.component';
import { ConfigComponent } from './app/config.component';
import { SimulatorComponent } from './app/simulator.component';

bootstrapApplication(AppComponent, {
  providers: [
    provideHttpClient(),
    provideRouter([
      { path: '', redirectTo: 'simulator', pathMatch: 'full' },
      { path: 'simulator', component: SimulatorComponent },
      { path: 'config', component: ConfigComponent }
    ])
  ]
}).catch(err => console.error(err));
