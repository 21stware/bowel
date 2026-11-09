import AVFoundation
import UIKit

/// A full-screen camera that hands back the first QR code it reads, or nil
/// when cancelled. Cancel sits at the bottom, within reach of a thumb.
final class ScannerController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
  private let onDone: (String?) -> Void
  private let session = AVCaptureSession()
  private let sessionQueue = DispatchQueue(label: "com.oboo.bowel.scanner")
  private let preview = AVCaptureVideoPreviewLayer()
  private let message = UILabel()
  private let settings = UIButton(type: .system)
  private var finished = false
  private let words: [String: String]

  /// `words` are the screen's text in the interface's language.
  init(words: [String: String], onDone: @escaping (String?) -> Void) {
    self.words = words
    self.onDone = onDone
    super.init(nibName: nil, bundle: nil)
  }

  @available(*, unavailable)
  required init?(coder: NSCoder) { fatalError("ScannerController is built in code") }

  override var preferredStatusBarStyle: UIStatusBarStyle { .lightContent }

  override func viewDidLoad() {
    super.viewDidLoad()
    view.backgroundColor = .black
    preview.session = session
    preview.videoGravity = .resizeAspectFill
    view.layer.addSublayer(preview)

    message.text = words["aim"] ?? "对准订阅或节点的二维码"
    message.textColor = .white
    message.font = .systemFont(ofSize: 16, weight: .medium)
    message.textAlignment = .center
    message.numberOfLines = 0
    message.translatesAutoresizingMaskIntoConstraints = false
    view.addSubview(message)

    var open = UIButton.Configuration.filled()
    open.title = words["settings"] ?? "打开系统设置"
    open.cornerStyle = .capsule
    open.baseBackgroundColor = .white
    open.baseForegroundColor = .black
    settings.configuration = open
    settings.isHidden = true
    settings.translatesAutoresizingMaskIntoConstraints = false
    settings.addAction(UIAction { _ in
      if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
    }, for: .touchUpInside)
    view.addSubview(settings)

    let cancel = UIButton(type: .system)
    var plain = UIButton.Configuration.filled()
    plain.title = words["cancel"] ?? "取消"
    plain.cornerStyle = .capsule
    plain.baseBackgroundColor = UIColor(white: 0.16, alpha: 0.9)
    plain.baseForegroundColor = .white
    cancel.configuration = plain
    cancel.translatesAutoresizingMaskIntoConstraints = false
    cancel.addAction(UIAction { [weak self] _ in self?.finish(nil) }, for: .touchUpInside)
    view.addSubview(cancel)

    let guide = view.safeAreaLayoutGuide
    NSLayoutConstraint.activate([
      message.leadingAnchor.constraint(equalTo: guide.leadingAnchor, constant: 32),
      message.trailingAnchor.constraint(equalTo: guide.trailingAnchor, constant: -32),
      message.bottomAnchor.constraint(equalTo: settings.topAnchor, constant: -16),
      settings.centerXAnchor.constraint(equalTo: guide.centerXAnchor),
      settings.heightAnchor.constraint(equalToConstant: 48),
      settings.bottomAnchor.constraint(equalTo: cancel.topAnchor, constant: -16),
      cancel.leadingAnchor.constraint(equalTo: guide.leadingAnchor, constant: 24),
      cancel.trailingAnchor.constraint(equalTo: guide.trailingAnchor, constant: -24),
      cancel.heightAnchor.constraint(equalToConstant: 52),
      cancel.bottomAnchor.constraint(equalTo: guide.bottomAnchor, constant: -20),
    ])

    switch AVCaptureDevice.authorizationStatus(for: .video) {
    case .authorized:
      startCamera()
    case .notDetermined:
      AVCaptureDevice.requestAccess(for: .video) { [weak self] granted in
        DispatchQueue.main.async {
          if granted { self?.startCamera() } else { self?.showDenied() }
        }
      }
    default:
      showDenied()
    }
  }

  override func viewDidLayoutSubviews() {
    super.viewDidLayoutSubviews()
    preview.frame = view.bounds
  }

  override func viewWillDisappear(_ animated: Bool) {
    super.viewWillDisappear(animated)
    let session = session
    sessionQueue.async { session.stopRunning() }
  }

  private func showDenied() {
    message.text = words["denied"] ?? "相机权限已关闭，请在系统设置中允许 bowel 使用相机"
    settings.isHidden = false
  }

  private func startCamera() {
    guard !finished else { return }
    guard let camera = AVCaptureDevice.default(for: .video),
      let input = try? AVCaptureDeviceInput(device: camera), session.canAddInput(input)
    else {
      message.text = words["noCamera"] ?? "这台设备没有可用的相机"
      return
    }
    session.addInput(input)
    let output = AVCaptureMetadataOutput()
    guard session.canAddOutput(output) else {
      message.text = words["noCamera"] ?? "这台设备没有可用的相机"
      return
    }
    session.addOutput(output)
    output.setMetadataObjectsDelegate(self, queue: .main)
    output.metadataObjectTypes = [.qr]
    let session = session
    sessionQueue.async { session.startRunning() }
  }

  func metadataOutput(
    _ output: AVCaptureMetadataOutput, didOutput objects: [AVMetadataObject], from connection: AVCaptureConnection
  ) {
    guard let code = objects.compactMap({ $0 as? AVMetadataMachineReadableCodeObject }).first?.stringValue,
      !code.isEmpty
    else { return }
    UINotificationFeedbackGenerator().notificationOccurred(.success)
    finish(code)
  }

  private func finish(_ code: String?) {
    guard !finished else { return }
    finished = true
    onDone(code)
  }
}
