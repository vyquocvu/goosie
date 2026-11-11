# Goosie Browser — Documentation Index

Tài liệu dự án Goosie được cấu trúc thành hai phân hệ rõ ràng:

---

## 🌟 1. Tài Liệu Chuẩn Master (Single Source of Truth - SSOT)
Toàn bộ định hướng sản phẩm, yêu cầu nghiệp vụ và thiết kế kiến trúc chính thức v2.0 (Day 1 Greenfield) được lưu trữ tập trung tại thư mục **[`docs/spec/`](spec/)**:

| Tài liệu | Vai trò phụ trách | Nội dung trọng tâm |
| :--- | :--- | :--- |
| **[`PRODUCT_VISION_AND_STRATEGY.md`](spec/PRODUCT_VISION_AND_STRATEGY.md)** | 👔 **Product Owner (PO)** | Tầm nhìn, Sứ mệnh, Đề xuất giá trị 10x, Personas, MoSCoW Backlog & Roadmap. |
| **[`SOFTWARE_REQUIREMENTS_SPECIFICATION.md`](spec/SOFTWARE_REQUIREMENTS_SPECIFICATION.md)** | 📋 **Business Analyst (BA)** | Đặc tả yêu cầu chức năng FR1-FR7, NFRs, Hard Admission Caps & User Stories (Gherkin). |
| **[`SYSTEM_ARCHITECTURE_BLUEPRINT.md`](spec/SYSTEM_ARCHITECTURE_BLUEPRINT.md)** | 🏛️ **Technical Architects (TAs)** | Bản thiết kế kiến trúc hợp nhất v2.0, Structs phẳng 64-byte, Process-Pool, Hybrid Rasterizer & 4 ADRs. |

> 📌 **Quy tắc:** Mọi công việc lập trình, review mã nguồn, phân chia task và nghiệm thu Sprint **BẮT BUỘC** phải căn cứ trên bộ tài liệu trong `docs/spec/`.

---

## 📚 2. Tư Liệu Thực Nghiệm & Lịch Sử Kiểm Thử (Empirical Test Data & History)
Các tài liệu dưới đây được giữ nguyên làm tư liệu đối chứng kỹ thuật, catalog kiểm thử và dữ liệu thực nghiệm đo đạc trên các website thực tế:

- **[`roadmap-v2.md`](roadmap-v2.md)**: Nhật ký đo đạc thực nghiệm, tiến độ kiểm thử WPT và ghi nhận các edge-cases trước đây.
- **[`qa-test-catalog.md`](qa-test-catalog.md)**: Danh mục kịch bản kiểm thử QA chi tiết.
- **[`qa-open-defects.json`](qa-open-defects.json)**: Cơ sở dữ liệu ghi nhận các sai lệch hiển thị (defects) thực tế so với Chromium.
- **[`engine-compatibility-review.md`](engine-compatibility-review.md)**: Báo cáo đánh giá tương thích với các framework web và thư viện DOM.
- **[`tasks/`](tasks/)**: Lịch sử các gói công việc kỹ thuật đã triển khai.
