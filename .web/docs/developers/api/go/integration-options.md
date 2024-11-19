<div class="integration-options">
  <a href="https://github.com/minekube/gate-plugin-template" class="option-card" target="_blank">
    <div class="option-content">
      <h3>🔌 Native Go Plugin</h3>
      <ul>
        <li>Create your own Go module and import Gate APIs</li>
        <li>Compile like a normal Go program</li>
        <li>Full access to Gate's core functionality</li>
        <li>Maximum performance with in-process execution</li>
        <li>Perfect for extending Gate's functionality</li>
      </ul>
      <div class="option-footer">
        <span class="action-link">Clone Starter Template →</span>
      </div>
    </div>
  </a>

  <a href="/developers/api/" class="option-card">
    <div class="option-content">
      <h3>🌐 HTTP API Client</h3>
      <ul>
        <li>Language-agnostic interface</li>
        <li>Perfect for out-of-process execution</li>
        <li>Independent deployment cycles</li>
        <li>Cross-version compatibility</li>
        <li>Ideal for external integrations</li>
      </ul>
      <div class="option-footer">
        <span class="action-link">Get Started →</span>
      </div>
    </div>
  </a>
</div>

<style>
.integration-options {
  display: flex;
  flex-direction: column;
  gap: 20px;
  margin: 24px 0;
  max-width: 800px;
}

.option-card {
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  transition: all 0.3s ease;
  text-decoration: none !important;
  color: inherit;
  display: block;
}

.option-content {
  padding: 20px;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.option-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 2px 12px 0 var(--vp-c-divider);
  border-color: var(--vp-c-brand-1);
}

.option-card h3 {
  margin-top: 0;
  margin-bottom: 16px;
  color: var(--vp-c-brand-1);
}

.option-card ul {
  padding-left: 20px;
  margin-bottom: 16px;
  flex-grow: 1;
}

.option-card li {
  margin: 8px 0;
  color: var(--vp-c-text-2);
}

.option-footer {
  margin-top: auto;
  padding-top: 16px;
  border-top: 1px solid var(--vp-c-divider);
}

.action-link {
  color: var(--vp-c-brand-1);
  font-weight: 500;
  display: inline-flex;
  align-items: center;
}

.option-card:hover .action-link {
  text-decoration: none;
}

.info-box {
  margin-top: 24px;
  padding: 16px 20px;
  background-color: var(--vp-c-bg-soft);
  border-left: 4px solid var(--vp-c-brand-1);
  border-radius: 0 8px 8px 0;
}

.info-box h4 {
  margin-top: 0;
  margin-bottom: 8px;
  color: var(--vp-c-brand-1);
}

.info-box p {
  margin: 0;
  color: var(--vp-c-text-2);
}
</style>
