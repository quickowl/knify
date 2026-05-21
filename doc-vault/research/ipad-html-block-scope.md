---
type: Note
status: Scope
related_to: "[[mobile-pairing]]"
---

# iPad html block scope

Scoping note for adding the `html` block kind to the iPad reviewer in `~/Prog/xcode/ipad-mux-2/apps/ios/`. Not yet implemented in that repo.

## Files to touch

1. `AgentCanvas/Models/CanvasModels.swift`
   - `enum CanvasBlockKind` (line 319): add `case html`.
   - `enum CanvasBlock` (line 338): add `case html(HtmlBlock)`. Update `var id`, `var kind`, and the `Encodable`/`Decodable` implementations alongside the other kinds. Add `HtmlBlock` struct with `id: String`, `html: String?`, `sandbox: HtmlSandbox?`, `height: Int?`, `screenshotAssetID: String?`, `screenshotURL: String?`, `title: String?`, `caption: String?`. Add `enum HtmlSandbox: String, Codable { case strict, relaxed }`.
   - `init(from decoder:)` (around line 481): add `case .html:` arm decoding either a `payload` envelope or the flat shape.

2. `AgentCanvas/Features/Canvas/CanvasRendererView.swift` (around line 260): add `case .html(let html): HtmlBlockView(block: html)` to the dispatch switch.

3. New file: `AgentCanvas/Features/Canvas/Blocks/HtmlBlockView.swift` — SwiftUI wrapper around a `WKWebView` for the inline path, with a fallback to `AsyncImage` for the screenshot path. See sketch below.

4. `AgentCanvasTests/UniversalBlockViewsTests.swift`: add a snapshot/golden test that decodes a canvas with an `html` block and confirms render (title + iframe substitute or image).

## SwiftUI sketch

```swift
import SwiftUI
import WebKit

struct HtmlBlockView: View {
    let block: HtmlBlock

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let title = block.title, !title.isEmpty {
                Text(title).font(.caption.monospaced()).foregroundStyle(.secondary)
            }
            if let html = block.html, !html.isEmpty {
                HtmlWebView(
                    html: html,
                    allowsScripts: block.sandbox == .relaxed,
                    initialHeight: clampedHeight(block.height)
                )
                .frame(height: clampedHeight(block.height))
                .background(Color(.systemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 6))
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(Color(.separator), lineWidth: 1))
            } else if let urlString = block.screenshotURL,
                      let url = URL(string: urlString) {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case .empty: ProgressView()
                    case .success(let image): image.resizable().aspectRatio(contentMode: .fit)
                    case .failure: HtmlBlockPlaceholder(text: block.title ?? "Screenshot")
                    @unknown default: EmptyView()
                    }
                }
            } else {
                HtmlBlockPlaceholder(text: "HTML block has no body or screenshot.")
            }
            if let caption = block.caption, !caption.isEmpty {
                Text(caption).font(.footnote).foregroundStyle(.secondary)
            }
        }
    }

    private func clampedHeight(_ raw: Int?) -> CGFloat {
        let value = CGFloat(raw ?? 320)
        return min(max(value, 80), 1600)
    }
}

private struct HtmlBlockPlaceholder: View {
    let text: String
    var body: some View {
        Text(text)
            .font(.callout.monospaced())
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity, minHeight: 120)
            .padding()
            .overlay(RoundedRectangle(cornerRadius: 6).stroke(style: StrokeStyle(lineWidth: 1, dash: [4])).foregroundStyle(.secondary))
    }
}

private struct HtmlWebView: UIViewRepresentable {
    let html: String
    let allowsScripts: Bool
    let initialHeight: CGFloat

    func makeUIView(context: Context) -> WKWebView {
        let preferences = WKWebpagePreferences()
        preferences.allowsContentJavaScript = allowsScripts
        let config = WKWebViewConfiguration()
        config.defaultWebpagePreferences = preferences
        config.suppressesIncrementalRendering = true
        let webView = WKWebView(frame: CGRect(x: 0, y: 0, width: 800, height: initialHeight), configuration: config)
        webView.isOpaque = false
        webView.backgroundColor = .clear
        webView.scrollView.isScrollEnabled = false
        webView.navigationDelegate = context.coordinator
        return webView
    }

    func updateUIView(_ uiView: WKWebView, context: Context) {
        uiView.loadHTMLString(html, baseURL: nil)
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    final class Coordinator: NSObject, WKNavigationDelegate {
        func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            // Strict: block any navigation that isn't the initial loadHTMLString.
            switch navigationAction.navigationType {
            case .linkActivated, .formSubmitted, .formResubmitted, .other where navigationAction.targetFrame == nil:
                decisionHandler(.cancel)
            default:
                decisionHandler(.allow)
            }
        }
    }
}
```

## Notes on the iOS sandbox model

`WKWebView` is the closest analogue to the web's iframe `srcdoc` + `sandbox`. There is no exact attribute-level equivalent of `sandbox=""`, but the two important levers are:

- `WKWebpagePreferences.allowsContentJavaScript` — set `false` for strict, `true` for relaxed. This is the closest match to the web viewer's strict-vs-relaxed split.
- `WKNavigationDelegate.decidePolicyFor` — cancel link activations, form submissions, and target-frame-less navigations so the html block cannot pop the reviewer into Safari or trigger an arbitrary URL load.
- `loadHTMLString(_, baseURL: nil)` gives the content an opaque origin, so it cannot read any shared state from the app.

## Verification

After implementation, point the iPad app at the local hub (`HUB_BASE_URL=http://127.0.0.1:8799`, dev bearer) and open `canvas-html-demo`. The "Inline html snippet" block should render the gradient card; the "Screenshot fallback" block should render the PNG (or its placeholder if the asset endpoint requires auth — same behavior as the web viewer).
