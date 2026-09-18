/**
 * MyAI Mobile UI Prototype - Toast Feedback Component
 */

export function showToast(message, duration = 2000) {
  let toast = document.getElementById("ui-toast");
  if (!toast) {
    toast = document.createElement("div");
    toast.id = "ui-toast";
    toast.style.cssText = `
      position: absolute;
      top: 70px;
      left: 50%;
      transform: translateX(-50%);
      background: #12100e;
      color: #fffdf7;
      padding: 8px 16px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 800;
      z-index: 1000;
      box-shadow: 0 4px 12px rgba(0,0,0,0.2);
      pointer-events: none;
      transition: opacity 0.2s ease, transform 0.2s ease;
      opacity: 0;
    `;
    document.querySelector(".screen-viewport")?.appendChild(toast);
  }

  toast.textContent = message;
  toast.style.opacity = "1";
  toast.style.transform = "translateX(-50%) translateY(0)";

  setTimeout(() => {
    toast.style.opacity = "0";
    toast.style.transform = "translateX(-50%) translateY(-10px)";
  }, duration);
}
