/* MyAI product site interactions. The demo uses static, fictional data. */
(() => {
  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];

  const nav = $("#nav");
  const toggle = $("#nav-toggle");
  const links = $("#nav-links");
  const onScroll = () => nav.classList.toggle("scrolled", window.scrollY > 8);
  onScroll();
  window.addEventListener("scroll", onScroll, { passive: true });
  const setMenu = (open) => {
    links.classList.toggle("open", open);
    nav.classList.toggle("menu-open", open);
    toggle.setAttribute("aria-expanded", String(open));
    toggle.setAttribute("aria-label", open ? "关闭菜单" : "打开菜单");
  };
  toggle.addEventListener("click", () => setMenu(!links.classList.contains("open")));
  $$("a", links).forEach((a) => a.addEventListener("click", () => setMenu(false)));
  nav.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && links.classList.contains("open")) {
      setMenu(false);
      toggle.focus();
    }
  });
  nav.classList.add("menu-ready");

  $$(".copy-btn").forEach((button) => {
    const text = button.closest("[data-copy]")?.dataset.copy ??
      $("pre code", button.parentElement)?.textContent.trim();
    if (!text) return;
    let reset;
    button.addEventListener("click", async () => {
      let copied = false;
      try {
        await navigator.clipboard.writeText(text);
        copied = true;
      } catch {
        const field = document.createElement("textarea");
        field.value = text;
        field.style.cssText = "position:fixed;opacity:0;";
        document.body.append(field);
        field.select();
        try { copied = document.execCommand("copy"); } catch { /* manual copy */ }
        field.remove();
        button.focus({ preventScroll: true });
      }
      clearTimeout(reset);
      button.textContent = copied ? "已复制" : "请手动复制";
      button.classList.toggle("copied", copied);
      reset = setTimeout(() => {
        button.textContent = "复制";
        button.classList.remove("copied");
      }, 1600);
    });
    button.hidden = false;
  });

  const tabs = $$(".tabs [role='tab']");
  const selectTab = (tab) => {
    tabs.forEach((candidate) => {
      const selected = candidate === tab;
      candidate.setAttribute("aria-selected", String(selected));
      candidate.tabIndex = selected ? 0 : -1;
      document.getElementById(candidate.getAttribute("aria-controls")).hidden = !selected;
    });
  };
  tabs.forEach((tab, index) => {
    tab.addEventListener("click", () => selectTab(tab));
    tab.addEventListener("keydown", (event) => {
      const target = {
        ArrowRight: (index + 1) % tabs.length,
        ArrowLeft: (index + tabs.length - 1) % tabs.length,
        Home: 0,
        End: tabs.length - 1,
      }[event.key];
      if (target === undefined) return;
      event.preventDefault();
      selectTab(tabs[target]);
      tabs[target].focus();
    });
  });
  selectTab(tabs[0]);
  $(".tabs").hidden = false;

  const demo = $("#product-demo");
  const terminal = $("#demo-terminal", demo);
  const mobile = $("#demo-mobile", demo);
  const status = $("#demo-status", demo);
  const reset = $("#demo-reset", demo);
  const steps = $$('[data-demo-scene]', demo);
  const action = (scene, text, quiet = false) =>
    `<button type="button" class="demo-action${quiet ? " demo-action-quiet" : ""}" data-demo-action="${scene}">${text}</button>`;
  const log = (label, text) => `<li><code>${label}</code><span>${text}</span></li>`;
  const taskTerminal = terminal.innerHTML;
  const taskMobile = mobile.innerHTML;
  const terminalScene = (title, entries) => `
    <p class="demo-terminal-path mono">~/work/retry-service</p>
    <h3>${title}</h3>
    <ul class="demo-log" aria-label="电脑端执行记录">${entries.map(([label, text]) => log(label, text)).join("")}</ul>`;
  const views = {
    task: {
      pc: taskTerminal,
      mobile: `${taskMobile}<div class="demo-actions">${action("approve", "查看权限请求 →")}</div>`,
      status: "任务通过 Relay 送到电脑，代码读写和测试由 PC Agent 执行。",
    },
    approve: {
      pc: terminalScene("等待手机审批", [
        ["write", "已更新 <code>internal/retry.go</code> 和回归测试"],
        ["shell", "请求运行 <code>go test ./...</code>"],
        ["pending", "收到审批结果前，测试命令不会运行"],
      ]),
      mobile: `<h3 tabindex="-1">要运行这个命令吗？</h3>
        <div class="demo-permission"><span>工具权限 · Shell</span><code>go test ./...</code></div>
        <p>这次调用会在电脑的项目目录里运行测试。</p>
        <div class="demo-actions">${action("review", "允许本次")}${action("denied", "拒绝", true)}</div>`,
      status: "是否需要审批由权限策略决定；这个示例中的测试命令需要你确认。",
    },
    review: {
      pc: terminalScene("测试通过，两处改动待检查", [
        ["approved", "手机已放行本次工具调用"],
        ["test", "<code>go test ./...</code> · PASS"],
        ["changes", "<code>internal/retry.go</code> 和 <code>internal/retry_test.go</code>"],
      ]),
      mobile: `<h3 tabindex="-1">变更 · 2 个文件</h3>
        <div class="demo-diff"><code>internal/retry.go</code><pre><span>− attempt &lt;= maxRetries</span><span>+ attempt &lt; maxRetries</span></pre></div>
        <p>选中的是重试逻辑文件。测试文件可以单独保留。</p>
        <div class="demo-actions">${action("reverted", "还原所选文件", true)}</div>`,
      status: "手机「变更」页可以按文件看 diff；可恢复的文件可以单独还原。",
    },
    reverted: {
      pc: terminalScene("已还原一个文件", [
        ["reverted", "<code>internal/retry.go</code> 已恢复至历史基线"],
        ["kept", "<code>internal/retry_test.go</code> 的改动仍在"],
      ]),
      mobile: `<h3 tabindex="-1">已还原所选文件</h3>
        <div class="demo-result"><strong>✓ 已还原</strong><code>internal/retry.go</code></div>
        <p>还有 1 个文件保留改动。文件彼此有依赖时，建议还原后重新跑测试。</p>
        <div class="demo-actions">${action("task", "从头再试 ↺", true)}</div>`,
      status: "演示结束：操作只还原了选中的文件。",
    },
    denied: {
      pc: terminalScene("本次命令未运行", [
        ["denied", "用户拒绝了 <code>go test ./...</code>"],
        ["changes", "审批之前写入的文件仍然保留"],
      ]),
      mobile: `<h3 tabindex="-1">已拒绝本次调用</h3>
        <p>测试未运行，之前已经写入的改动可以到「变更」页检查。</p>
        <div class="demo-actions">${action("approve", "重试审批", true)}</div>`,
      status: "拒绝只阻止当前工具调用，不会自动撤销之前的文件操作。",
    },
  };
  const showScene = (scene, focus = false) => {
    const view = views[scene];
    demo.dataset.scene = scene;
    terminal.innerHTML = view.pc;
    mobile.innerHTML = view.mobile;
    status.textContent = view.status;
    steps.forEach((button) => button.setAttribute("aria-pressed", String(button.dataset.demoScene === (scene === "denied" ? "approve" : scene))));
    if (focus) $("h3", mobile).focus({ preventScroll: true });
  };
  demo.addEventListener("click", (event) => {
    const button = event.target.closest("button");
    if (!button) return;
    if (button.dataset.demoScene) showScene(button.dataset.demoScene);
    if (button.dataset.demoAction) showScene(button.dataset.demoAction, true);
  });
  reset.addEventListener("click", () => {
    showScene("task");
    steps[0].focus();
  });
  showScene("task");
  $("#demo-steps").hidden = false;
  reset.hidden = false;
})();
