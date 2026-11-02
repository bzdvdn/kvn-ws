(function () {
  var year = document.getElementById("year");
  if (year) year.textContent = String(new Date().getFullYear());

  var toggle = document.getElementById("menu-toggle");
  var nav = document.getElementById("site-nav");
  if (toggle && nav) {
    toggle.addEventListener("click", function () {
      var open = nav.classList.toggle("open");
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  var passwordButton = document.getElementById("toggle-password");
  var password = document.getElementById("password");
  if (passwordButton && password) {
    passwordButton.addEventListener("click", function () {
      var show = password.type === "password";
      password.type = show ? "text" : "password";
      passwordButton.textContent = show ? "Hide" : "Show";
      passwordButton.setAttribute("aria-label", show ? "Hide password" : "Show password");
    });
  }

  var form = document.getElementById("signin-form");
  var error = document.getElementById("signin-error");
  var submit = document.getElementById("signin-submit");
  if (form && error && submit) {
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      error.hidden = true;
      var email = document.getElementById("email");
      var pass = document.getElementById("password");
      if (!email || !pass || !email.value || !pass.value) {
        error.textContent = "Enter your email and password.";
        error.hidden = false;
        return;
      }
      submit.disabled = true;
      submit.textContent = "Signing in…";
      setTimeout(function () {
        error.textContent = "Invalid email or password.";
        error.hidden = false;
        submit.disabled = false;
        submit.textContent = "Sign in";
      }, 700);
    });
  }
})();
