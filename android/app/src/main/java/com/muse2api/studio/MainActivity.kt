package com.muse2api.studio

import android.annotation.SuppressLint
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.view.ViewGroup
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import com.muse2api.studio.mobile.Mobile // gomobile 生成：javapkg + Go 包名 mobile
import java.io.File

// 壳：启动 Go 本地服务（只绑 127.0.0.1），用 WebView 打开同一份 Studio 页面。
// 密钥存在应用内部存储的 config.json，不进浏览器、不出本机。
class MainActivity : ComponentActivity() {
    private lateinit var webView: WebView
    private var fileCallback: ValueCallback<Array<Uri>>? = null

    // 改图 / 对话页的图片上传：系统选图器（多选），结果回传给 WebView
    private val pickFiles =
        registerForActivityResult(ActivityResultContracts.GetMultipleContents()) { uris ->
            fileCallback?.onReceiveValue(uris.toTypedArray())
            fileCallback = null
        }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        webView = WebView(this).apply {
            layoutParams = ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT,
            )
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.allowFileAccess = true
            settings.loadWithOverviewMode = true
            settings.useWideViewPort = true
            webViewClient = object : WebViewClient() {
                override fun shouldOverrideUrlLoading(
                    view: WebView,
                    request: WebResourceRequest,
                ): Boolean {
                    // 本地服务留在 WebView 里；http(s) 外链（GitHub / LINUX DO）交给系统浏览器；
                    // 其他 scheme（about:blank 等）不管
                    val host = request.url.host
                    val scheme = request.url.scheme
                    return if (host == "127.0.0.1") {
                        false
                    } else if (scheme == "http" || scheme == "https") {
                        startActivity(Intent(Intent.ACTION_VIEW, request.url))
                        true
                    } else {
                        false
                    }
                }
            }
            webChromeClient = object : WebChromeClient() {
                override fun onShowFileChooser(
                    view: WebView,
                    callback: ValueCallback<Array<Uri>>,
                    params: FileChooserParams,
                ): Boolean {
                    fileCallback?.onReceiveValue(null)
                    fileCallback = callback
                    pickFiles.launch("image/*")
                    return true
                }
            }
        }
        setContentView(webView)

        Thread {
            val dir = File(filesDir, "studio").apply { mkdirs() }
            val port = Mobile.start(dir.absolutePath)
            runOnUiThread {
                if (port <= 0) {
                    Toast.makeText(this, "本地服务启动失败", Toast.LENGTH_LONG).show()
                } else {
                    webView.loadUrl("http://127.0.0.1:$port/")
                }
            }
        }.start()
    }

    @Deprecated("改用返回键处理 WebView 历史")
    override fun onBackPressed() {
        if (::webView.isInitialized && webView.canGoBack()) {
            webView.goBack()
        } else {
            super.onBackPressed()
        }
    }
}
