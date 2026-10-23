package __JAVA_PACKAGE__;

import android.annotation.SuppressLint;
import android.graphics.Color;
import android.os.Looper;
import android.util.LongSparseArray;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;

import java.util.concurrent.ExecutionException;
import java.util.concurrent.FutureTask;

final class SystemWebView implements AutoCloseable {
	private static final int COMMAND_CREATE = 1;
	private static final int COMMAND_NAVIGATE = 2;
	private static final int COMMAND_SET_HTML = 3;
	private static final int COMMAND_EVAL = 4;
	private static final int COMMAND_SET_BOUNDS = 5;
	private static final int COMMAND_SET_VISIBLE = 6;
	private static final int COMMAND_DESTROY = 7;

	private static final int EVENT_READY = 1;
	private static final int EVENT_NAVIGATION = 2;
	private static final int EVENT_ERROR = 3;
	private static final int EVENT_CLOSED = 4;

	private static final int FLAG_VISIBLE = 1;
	private static final int FLAG_DEV_TOOLS = 1 << 1;
	private static final int FLAG_TRANSPARENT = 1 << 2;
	private static final int FLAG_HTML = 1 << 3;

	private final FrameLayout rootView;
	private final LongSparseArray<Entry> entries = new LongSparseArray<>();

	SystemWebView(FrameLayout rootView) {
		this.rootView = rootView;
	}

	void sync() {
		while (true) {
			long packed = NativeBridge.webViewCommand();
			long sequence = packed >>> 8;
			if (sequence == 0) return;
			try {
				dispatch((int)(packed & 0xffL), NativeBridge.webViewCommandID());
			} catch (RuntimeException error) {
				NativeBridge.webViewEvent(NativeBridge.webViewCommandID(), EVENT_ERROR, error.toString());
			} finally {
				NativeBridge.webViewCommandComplete(sequence);
			}
		}
	}

	private void dispatch(int command, long id) {
		switch (command) {
			case COMMAND_CREATE:
				create(id, NativeBridge.webViewCommandFlags(), NativeBridge.webViewCommandText());
				break;
			case COMMAND_NAVIGATE:
				require(id).view.loadUrl(NativeBridge.webViewCommandText());
				break;
			case COMMAND_SET_HTML:
				loadHTML(require(id).view, NativeBridge.webViewCommandText());
				break;
			case COMMAND_EVAL:
				require(id).view.evaluateJavascript(NativeBridge.webViewCommandText(), null);
				break;
			case COMMAND_SET_BOUNDS:
				setBounds(require(id).view);
				break;
			case COMMAND_SET_VISIBLE:
				require(id).view.setVisibility((NativeBridge.webViewCommandFlags() & FLAG_VISIBLE) != 0 ? WebView.VISIBLE : WebView.GONE);
				break;
			case COMMAND_DESTROY:
				destroy(id, true);
				break;
			default:
				throw new IllegalArgumentException("Unknown WebView command " + command);
		}
	}

	@SuppressLint("SetJavaScriptEnabled")
	private void create(long id, int flags, String content) {
		if (entries.get(id) != null) throw new IllegalStateException("WebView " + id + " already exists");
		WebView view = new WebView(rootView.getContext());
		WebSettings settings = view.getSettings();
		settings.setJavaScriptEnabled(true);
		settings.setDomStorageEnabled(true);
		view.addJavascriptInterface(new OnixBridge(id), "onix");
		Entry entry = new Entry(view);
		view.setWebViewClient(new WebViewClient() {
			@Override public void onPageFinished(WebView source, String url) {
				NativeBridge.webViewEvent(id, EVENT_NAVIGATION, url == null ? "" : url);
				if (!entry.ready) {
					entry.ready = true;
					NativeBridge.webViewEvent(id, EVENT_READY, "");
				}
			}

			@Override public void onReceivedError(WebView source, WebResourceRequest request, WebResourceError error) {
				if (!request.isForMainFrame()) return;
				CharSequence description = error.getDescription();
				NativeBridge.webViewEvent(id, EVENT_ERROR, description == null ? "WebView navigation failed" : description.toString());
			}
		});
		if ((flags & FLAG_DEV_TOOLS) != 0) WebView.setWebContentsDebuggingEnabled(true);
		if ((flags & FLAG_TRANSPARENT) != 0) view.setBackgroundColor(Color.TRANSPARENT);
		view.setVisibility((flags & FLAG_VISIBLE) != 0 ? WebView.VISIBLE : WebView.GONE);
		entries.put(id, entry);
		rootView.addView(view, new FrameLayout.LayoutParams(0, 0));
		setBounds(view);
		if ((flags & FLAG_HTML) != 0) loadHTML(view, content);
		else view.loadUrl(content);
	}

	private void loadHTML(WebView view, String html) {
		view.loadDataWithBaseURL("https://onix.invalid/", html, "text/html", "UTF-8", null);
	}

	private void setBounds(WebView view) {
		FrameLayout.LayoutParams params = (FrameLayout.LayoutParams)view.getLayoutParams();
		params.width = Math.max(0, Math.round(NativeBridge.webViewCommandWidth()));
		params.height = Math.max(0, Math.round(NativeBridge.webViewCommandHeight()));
		params.leftMargin = Math.round(NativeBridge.webViewCommandX());
		params.topMargin = Math.round(NativeBridge.webViewCommandY());
		view.setLayoutParams(params);
	}

	private Entry require(long id) {
		Entry entry = entries.get(id);
		if (entry == null) throw new IllegalStateException("WebView " + id + " does not exist");
		return entry;
	}

	private String invokeOnMainThread(long id, String name, String payload) {
		if (Looper.myLooper() == Looper.getMainLooper()) {
			return NativeBridge.webViewInvoke(id, name, payload);
		}
		FutureTask<String> task = new FutureTask<>(() -> NativeBridge.webViewInvoke(id, name, payload));
		if (!rootView.post(task)) throw new IllegalStateException("WebView host is closed");
		try {
			return task.get();
		} catch (InterruptedException error) {
			Thread.currentThread().interrupt();
			throw new IllegalStateException("WebView invocation was interrupted", error);
		} catch (ExecutionException error) {
			Throwable cause = error.getCause();
			if (cause instanceof RuntimeException) throw (RuntimeException)cause;
			throw new IllegalStateException("WebView invocation failed", cause);
		}
	}

	private void destroy(long id, boolean notify) {
		Entry entry = entries.get(id);
		if (entry == null) return;
		entries.remove(id);
		entry.view.removeJavascriptInterface("onix");
		rootView.removeView(entry.view);
		entry.view.destroy();
		if (notify) NativeBridge.webViewEvent(id, EVENT_CLOSED, "");
	}

	@Override public void close() {
		for (int index = entries.size() - 1; index >= 0; index--) {
			destroy(entries.keyAt(index), false);
		}
	}

	private static final class Entry {
		final WebView view;
		boolean ready;

		Entry(WebView view) {
			this.view = view;
		}
	}

	private final class OnixBridge {
		private final long id;

		OnixBridge(long id) {
			this.id = id;
		}

		@JavascriptInterface
		public String invoke(String name, String payload) {
			return invokeOnMainThread(id, name == null ? "" : name, payload == null ? "" : payload);
		}
	}
}
