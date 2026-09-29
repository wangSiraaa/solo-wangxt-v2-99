import { Component } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  template: `
    <header class="topbar">
      <div>
        <h1>可解释实验分流平台</h1>
        <p>稳定哈希 · 互斥层 · 半开桶区间 · 曝光与白名单原因分离</p>
      </div>
      <nav>
        <a routerLink="/simulator" routerLinkActive="active">分流解释</a>
        <a routerLink="/config" routerLinkActive="active">实验配置</a>
      </nav>
    </header>
    <main class="container">
      <router-outlet />
    </main>
  `
})
export class AppComponent {}
