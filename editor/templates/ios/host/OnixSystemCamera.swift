import AVFoundation
import CoreVideo
import Foundation

final class OnixSystemCamera: NSObject, AVCaptureVideoDataOutputSampleBufferDelegate {
	private static let commandOpen: UInt64 = 1
	private static let commandClose: UInt64 = 2
	private static let eventOpened: Int32 = 1
	private static let eventClosed: Int32 = 2
	private static let eventError: Int32 = 3
	private static let frameInterval = 0.18

	private let session = AVCaptureSession()
	private let queue = DispatchQueue(label: "onix.camera.capture")
	private var commandSequence: UInt64 = 0
	private var output: AVCaptureVideoDataOutput?
	private var lastFrameTime = 0.0

	func sync() {
		let packed = UInt64(OnixCameraCommand())
		let sequence = packed >> 8
		guard sequence != 0, sequence != commandSequence else { return }
		commandSequence = sequence
		switch packed & 0xff {
		case Self.commandOpen: requestOpen()
		case Self.commandClose: close(notify: true)
		default: break
		}
	}

	private func requestOpen() {
		switch AVCaptureDevice.authorizationStatus(for: .video) {
		case .authorized: configure()
		case .notDetermined:
			AVCaptureDevice.requestAccess(for: .video) { [weak self] granted in
				if granted { self?.configure() }
				else { OnixCameraEvent(Self.eventError) }
			}
		default: OnixCameraEvent(Self.eventError)
		}
	}

	private func configure() {
		queue.async { [weak self] in
			guard let self else { return }
			do {
				self.stopSession()
				let position: AVCaptureDevice.Position = OnixCameraFacingMode() == 2 ? .front : .back
				let discovery = AVCaptureDevice.DiscoverySession(
					deviceTypes: [.builtInWideAngleCamera], mediaType: .video, position: position
				)
				guard let device = discovery.devices.first else { throw CameraError.unavailable }
				let input = try AVCaptureDeviceInput(device: device)
				let output = AVCaptureVideoDataOutput()
				output.alwaysDiscardsLateVideoFrames = true
				output.videoSettings = [kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA]
				output.setSampleBufferDelegate(self, queue: self.queue)
				guard self.session.canAddInput(input), self.session.canAddOutput(output) else { throw CameraError.configuration }
				self.session.beginConfiguration()
				self.session.sessionPreset = .high
				self.session.addInput(input)
				self.session.addOutput(output)
				self.output = output
				self.session.commitConfiguration()
				self.session.startRunning()
				OnixCameraEvent(Self.eventOpened)
			} catch {
				if self.session.isRunning { self.session.stopRunning() }
				OnixCameraEvent(Self.eventError)
			}
		}
	}

	func captureOutput(_ output: AVCaptureOutput, didOutput sampleBuffer: CMSampleBuffer, from connection: AVCaptureConnection) {
		let now = CFAbsoluteTimeGetCurrent()
		guard now - lastFrameTime >= Self.frameInterval,
			let buffer = CMSampleBufferGetImageBuffer(sampleBuffer) else { return }
		lastFrameTime = now
		CVPixelBufferLockBaseAddress(buffer, .readOnly)
		defer { CVPixelBufferUnlockBaseAddress(buffer, .readOnly) }
		guard let base = CVPixelBufferGetBaseAddress(buffer) else { return }
		let width = CVPixelBufferGetWidth(buffer)
		let height = CVPixelBufferGetHeight(buffer)
		let stride = CVPixelBufferGetBytesPerRow(buffer)
		let maximumWidth = max(Int(OnixCameraWidth()), 1)
		let maximumHeight = max(Int(OnixCameraHeight()), 1)
		let scale = min(1, min(Double(maximumWidth) / Double(width), Double(maximumHeight) / Double(height)))
		let outputWidth = max(Int(Double(width) * scale), 1)
		let outputHeight = max(Int(Double(height) * scale), 1)
		let source = base.assumingMemoryBound(to: UInt8.self)
		var rgba = [UInt8](repeating: 0, count: outputWidth * outputHeight * 4)
		for y in 0..<outputHeight {
			for x in 0..<outputWidth {
				let sourceX = x * width / outputWidth
				let sourceY = y * height / outputHeight
				let input = sourceY * stride + sourceX * 4
				let output = (y * outputWidth + x) * 4
				rgba[output] = source[input + 2]
				rgba[output + 1] = source[input + 1]
				rgba[output + 2] = source[input]
				rgba[output + 3] = source[input + 3]
			}
		}
		rgba.withUnsafeMutableBufferPointer { pixels in
			OnixCameraFrame(pixels.baseAddress, Int32(rgba.count), Int32(outputWidth), Int32(outputHeight))
		}
	}

	func close(notify: Bool = false) {
		queue.async { [weak self] in
			self?.stopSession()
			if notify { OnixCameraEvent(Self.eventClosed) }
		}
	}

	private func stopSession() {
		if session.isRunning { session.stopRunning() }
		output?.setSampleBufferDelegate(nil, queue: nil)
		output = nil
		session.beginConfiguration()
		for input in session.inputs { session.removeInput(input) }
		for output in session.outputs { session.removeOutput(output) }
		session.commitConfiguration()
		lastFrameTime = 0
	}

	private enum CameraError: Error {
		case unavailable
		case configuration
	}
}
