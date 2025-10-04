document.addEventListener("DOMContentLoaded", () => {
  const form = document.getElementById("registerForm");
  if (form) {
    form.addEventListener("submit", (e) => {
      const p1 = document.getElementById("regPass");
      const p2 = document.getElementById("regPass2");
      const box = document.getElementById("regError");
      if (!p1 || !p2 || !box) return;
      if (p1.value.length < 6) {
        e.preventDefault();
        box.style.display = "block";
        box.textContent = "Password must be at least 6 characters.";
        p1.focus();
        return;
      }
      if (p1.value !== p2.value) {
        e.preventDefault();
        box.style.display = "block";
        box.textContent = "Passwords do not match.";
        p2.focus();
      }
    });
  }
  // ====== Post create form (NUEVO) ======
  const postForm = document.querySelector('form[action="/post/create"]');
  if (postForm) {
    postForm.addEventListener("submit", (e) => {
      const checks = postForm.querySelectorAll('input[name="cats"]:checked');
      const newCatEl = postForm.querySelector('input[name="newcat"]');
      const hasNew = newCatEl && newCatEl.value.trim() !== "";

      if (checks.length === 0 && !hasNew) {
        e.preventDefault();

        const box = document.getElementById("postError"); // <-- NUEVO
        const msg =
          "Please pick at least one category or create a new category.";

        if (box) {
          box.textContent = msg;
          box.style.display = "block";
          box.setAttribute("role", "alert");
        } else {
          alert(msg);
        }
        if (newCatEl) newCatEl.focus();
      }
    });
  }
});
