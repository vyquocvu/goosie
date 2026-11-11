# Goosie Browser — Documentation Index

Tài liệu dự án Goosie được cấu trúc thành hai phân hệ rõ ràng:

---

## 🌟 1. Tài Liệu Yêu Cầu & Kiến Trúc (Requirements & Architecture Reference)
Toàn bộ định hướng sản phẩm, yêu cầu nghiệp vụ và thiết kế kiến trúc chính thức v2.0 (Day 1 Greenfield) được lưu trữ tập trung tại thư mục **[`docs/spec/`](spec/)**:

| Tài liệu | Vai trò phụ trách | Nội dung trọng tâm |
| :--- | :--- | :--- |
| **[`PRODUCT_VISION_AND_STRATEGY.md`](spec/PRODUCT_VISION_AND_STRATEGY.md)** | 👔 **Product Owner (PO)** | Tầm nhìn, Sứ mệnh, Đề xuất giá trị 10x, Personas, MoSCoW Backlog & Roadmap. |
| **[`SOFTWARE_REQUIREMENTS_SPECIFICATION.md`](spec/SOFTWARE_REQUIREMENTS_SPECIFICATION.md)** | 📋 **Business Analyst (BA)** | Đặc tả yêu cầu chức năng FR1-FR7, NFRs, Hard Admission Caps & User Stories (Gherkin). |
| **[`SYSTEM_ARCHITECTURE_BLUEPRINT.md`](spec/SYSTEM_ARCHITECTURE_BLUEPRINT.md)** | 🏛️ **Technical Architects (TAs)** | Bản thiết kế kiến trúc hợp nhất v2.0, Structs phẳng 64-byte, Process-Pool, Hybrid Rasterizer & 4 ADRs. |
| **[`SPRINT_PLAN_AND_WBS.md`](spec/SPRINT_PLAN_AND_WBS.md)** | 🗓️ **Delivery Plan** | Phân rã công việc (WBS) và kế hoạch sprint tham chiếu. |

> 📌 **Quy tắc:** `docs/spec/` là bộ yêu cầu sản phẩm và kiến trúc tham chiếu (SRS, tầm nhìn, blueprint, WBS). Thẩm quyền lập kế hoạch và nghiệm thu hiện hành là **[`roadmap-v2.md`](roadmap-v2.md)** — trạng thái hiện tại và các cổng phát hành theo thứ tự, như quy định trong `AGENTS.md`. Không lập kế hoạch lại những gì roadmap đã ghi là hoàn thành.

---

## 🧭 Thẩm quyền thực thi (Execution Authority)

- **[`roadmap-v2.md`](roadmap-v2.md)**: đánh giá mức sẵn sàng sản xuất hiện hành và các cổng phát hành theo thứ tự (P0-P3). Đọc trước khi lập kế hoạch tính năng mới; không lên lại phần đã ghi là hoàn thành.

---

## 📚 2. Tư Liệu Thực Nghiệm & Lịch Sử Kiểm Thử (Empirical Test Data & History)
Các tài liệu dưới đây được giữ nguyên làm tư liệu đối chứng kỹ thuật, catalog kiểm thử và dữ liệu thực nghiệm đo đạc trên các website thực tế:

- **[`qa-test-catalog.md`](qa-test-catalog.md)**: Danh mục kịch bản kiểm thử QA chi tiết.
- **[`qa-open-defects.json`](qa-open-defects.json)**: Cơ sở dữ liệu ghi nhận các sai lệch hiển thị (defects) thực tế so với Chromium.
- **[`engine-compatibility-review.md`](engine-compatibility-review.md)**: Báo cáo đánh giá tương thích với các framework web và thư viện DOM.
- **[`tasks/`](tasks/)**: Lịch sử các gói công việc kỹ thuật đã triển khai.
