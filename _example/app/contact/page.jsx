const { h, useState, useRouter } = window.Lungo;

export const metadata = { title: "Contact — Lungo", description: "Get in touch using server actions." };

export default function ContactPage() {
  const router = useRouter();
  const params = new URLSearchParams(router.query);
  const success = params.get("success");
  const error = params.get("error");
  const [sending, setSending] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSending(true);
    const form = e.target;
    const data = new FormData(form);

    try {
      const res = await fetch("/action/contact", {
        method: "POST",
        headers: { "Accept": "application/json" },
        body: new URLSearchParams(data),
      });
      const result = await res.json();
      if (result.error) {
        router.replace("/contact?error=" + encodeURIComponent(result.error));
      } else if (result.redirect) {
        router.push(result.redirect);
      }
    } catch (err) {
      router.replace("/contact?error=Something went wrong");
    }
    setSending(false);
  };

  if (success) {
    return (
      <div>
        <h1 class="text-4xl font-extrabold tracking-tight mb-4 text-gray-900 dark:text-white">Thank You!</h1>
        <div class="bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-800 rounded-xl p-6 mb-6">
          <p class="text-green-700 dark:text-green-400">Your message has been sent successfully.</p>
        </div>
        <a href="/contact" class="text-blue-600 dark:text-blue-400 hover:underline">Send another message</a>
      </div>
    );
  }

  return (
    <div>
      <h1 class="text-4xl font-extrabold tracking-tight mb-4 text-gray-900 dark:text-white">Contact</h1>
      <p class="text-lg text-gray-500 dark:text-gray-400 mb-8 max-w-2xl">
        This form uses a Server Action — the POST goes directly to a Go handler, no API route needed.
      </p>

      {error ? (
        <div class="bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-xl p-4 mb-6 text-red-700 dark:text-red-400 text-sm">
          {decodeURIComponent(error)}
        </div>
      ) : null}

      <form onsubmit={handleSubmit} class="max-w-lg flex flex-col gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Name</label>
          <input
            name="name"
            type="text"
            class="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 text-gray-900 dark:text-white rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            placeholder="Your name"
          />
        </div>

        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Email</label>
          <input
            name="email"
            type="email"
            required="true"
            class="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 text-gray-900 dark:text-white rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            placeholder="you@example.com"
          />
        </div>

        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Message</label>
          <textarea
            name="message"
            rows="4"
            class="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 text-gray-900 dark:text-white rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            placeholder="Your message..."
          ></textarea>
        </div>

        <button
          type="submit"
          class="px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors font-medium disabled:opacity-50"
          disabled={sending}
        >
          {sending ? "Sending..." : "Send Message"}
        </button>
      </form>
    </div>
  );
}
